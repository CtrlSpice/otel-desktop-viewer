import test from 'node:test'
import assert from 'node:assert/strict'
import { decodeInspectProbe, decodeKit, decodePromptfoo, projectKitReport } from './offline-converters.ts'

// Small constructed artifacts; provenance and exclusions: offline-converters.md.
const promptfoo = () => ({
  evalId: 'eval-source-id', config: { prompts: ['private prompt'] },
  results: { version: 3, results: [
    { id: 'assertion', testIdx: 0, promptIdx: 0, success: false, failureReason: 1,
      score: 0.5, error: 'Assertion explanation, not a crash', cost: 0,
      gradingResult: { pass: false, score: 0.5, reason: 'original explanation',
        componentResults: [{ pass: true, score: 1, assertion: { type: 'contains', weight: 2 } },
          { pass: false, score: 0, namedScores: { correctness: 0 } }] } },
    { id: 'crash', success: false, failureReason: 2, score: 0, error: 'provider failed', gradingResult: null },
    { id: 'missing', gradingResult: null },
    { id: 'null-score', score: null },
  ] },
})

test('Promptfoo preserves envelope, score tree, null/absent/zero and unknown currency', () => {
  const input = promptfoo(), before = structuredClone(input)
  const result = decodePromptfoo(input)
  assert.deepEqual(result.source, input)
  assert.equal(result.rows[0].classification, 'assessment-failed')
  assert.equal(result.rows[1].classification, 'execution-error')
  assert.equal(result.rows[2].classification, 'unspecified')
  assert.equal(result.rows[0].source.cost, 0)
  assert.equal(Object.hasOwn(result.rows[0].source, 'currency'), false)
  assert.equal(Object.hasOwn(result.rows[2].source, 'score'), false)
  assert.equal(result.rows[1].source.gradingResult, null)
  assert.equal(result.rows[3].source.score, null)
  input.results.results[0].error = 'later mutation'
  assert.deepEqual(result.source, before)
  assert.deepEqual(decodePromptfoo(before), decodePromptfoo(before))
})

test('Promptfoo rejects wrong envelope/version and malformed discriminators instead of guessing', () => {
  const input = promptfoo()
  assert.throws(() => decodePromptfoo(input.results.results), /object/)
  assert.throws(() => decodePromptfoo({ results: input.results }), /evalId/)
  assert.throws(() => decodePromptfoo({ ...input, results: { version: 4, results: [] } }), /version 3/)
  assert.throws(() => decodePromptfoo({ ...input, results: { version: 3, outputs: [] } }), /array/)
  for (const change of [{ failureReason: '2' }, { failureReason: 3 }, { score: '0' }, { repeatIndex: -1 }, { success: null }]) {
    assert.throws(() => decodePromptfoo({ ...input, results: { version: 3, results: [change] } }))
  }
})

function kit() {
  return {
    manifest: { models: ['test/model'], tasks: 1, repetitions: 3, retryOf: 'retained-source-reference', telemetryViewer: '' },
    pilot: { results: { results: [
      { testIdx: 0, repeatIndex: 2, provider: { label: 'test/model' }, vars: { task_id: 'question' },
        success: false, failureReason: 1, response: { output: 'PRIVATE_SENTINEL', metadata: {
          model: 'test/model', sessionIDs: ['source-session'], evidence: '/not/read',
          telemetry: { traceID: 'a'.repeat(32), spanID: 'b'.repeat(16), tools: [
            { spanID: 'c'.repeat(16), name: 'tool source-name', error: true },
          ] },
        } }, gradingResult: { score: 0, componentResults: [{ metadata: { criteria: [
          { verdict: 'Fail', explanation: 'PRIVATE_SENTINEL' },
        ], role: 'grader', metadata: { telemetry: { traceID: 'd'.repeat(32), spanID: 'e'.repeat(16) } } } }] } },
      { provider: { label: 'test/model' }, vars: { task_id: 'question' }, failureReason: 2,
        response: { metadata: { sessionIDs: ['error-session'] } }, gradingResult: { score: 0 } },
    ] } },
    reviewEvidence: { revision: 2, previousHash: 'original', signedOff: true },
    deferredSlots: [{ model: 'test/model', task: 'question', repetition: 1 }],
  }
}

test('kit keeps original slots, recovery and raw evidence separate from signed report projection', () => {
  const input = kit(), before = structuredClone(input), decoded = decodeKit(input)
  const report = projectKitReport(decoded, [{ model: 'test/model', task: 'question',
    sessionIDs: ['source-session'], score: 1, signedOff: true }])
  assert.deepEqual(decoded.source, before)
  assert.equal(decoded.manifest.retryOf, 'retained-source-reference')
  assert.equal(decoded.rows[0].repeatIndex, 2)
  const result = report.models[0].result
  assert.equal(result.displayPass, 1)
  assert.equal(result.displayMean, 1 / 3)
  assert.equal(result.scoredAttempts, 1)
  assert.equal(result.error, 1)
  assert.equal(result.provisionalMean, 0)
  assert.equal(result.apiCostCad, null)
  assert.equal(result.estimateCoverage, 'unavailable')
  const task = report.models[0].tasks[0]
  assert.equal(task.runResults[0].telemetry?.traceID, 'a'.repeat(32))
  assert.equal(task.runResults[0].telemetry?.tools[0].spanID, 'c'.repeat(16))
  assert.equal(task.runResults[0].graderTelemetry?.spanID, 'e'.repeat(16))
  assert.equal(task.runResults[1].telemetry, null)
  assert.doesNotMatch(JSON.stringify(report), /PRIVATE_SENTINEL|\/not\/read/)
  assert.deepEqual(input, before)
  assert.equal((decoded.rows[0].gradingResult as { score: number }).score, 0)
})

test('kit refuses unsupported version and keeps absent optional artifacts absent', () => {
  const input = kit()
  assert.throws(() => decodeKit({ ...input, pilot: { results: { version: 4, results: [] } } }), /version/)
  const result = decodeKit(input)
  assert.equal(Object.hasOwn(result.source, 'pricing'), false)
  assert.deepEqual(projectKitReport(result), projectKitReport(result))
})

test('Inspect probe preserves categorical, structured, boolean, null and edited score evidence', () => {
  const input = { version: 2, eval: { eval_id: 'native-eval', run_id: 'native-run' }, samples: [
    { id: 'sample', epoch: 2, scores: {
      match: { value: 'I', answer: 'No' },
      structured: { value: { correct: true, score: 0, unknown: null, labels: ['a', 'b'] },
        history: [{ value: { correct: false }, explanation: 'original' }], explanation: 'edited' },
      missing: { value: null },
    }, error: { message: 'tool failed' } },
    { id: 2 },
  ] }
  const before = structuredClone(input), result = decodeInspectProbe(input)
  assert.deepEqual(result.source, before)
  assert.equal(result.samples[0].scores?.[0].source.value, 'I')
  assert.deepEqual(result.samples[0].scores?.[1].source, input.samples[0].scores!.structured)
  assert.equal(result.samples[1].scores, undefined)
  assert.equal(Object.hasOwn(result.samples[0], 'telemetry'), false)
  input.samples[0].scores!.match.value = 'C'
  assert.deepEqual(result.source, before)
})

test('finite JSON boundary rejects nonfinite, cycles and non-JSON objects without replacement', () => {
  for (const value of [NaN, Infinity, Number.MAX_SAFE_INTEGER + 1, undefined, 1n, new Date()]) {
    assert.throws(() => decodeInspectProbe({ version: 2, eval: {}, samples: [], extra: value }), /finite JSON/)
  }
  const cyclic: Record<string, unknown> = {}; cyclic.self = cyclic
  assert.throws(() => decodeKit(cyclic), /Cyclic/)
  assert.throws(() => decodeInspectProbe({ version: 3, eval: {}, samples: [] }), /version 2/)
  assert.throws(() => decodeInspectProbe({ version: 2, eval: {}, samples: [{ id: 1, scores: { match: {} } }] }), /missing value/)
})
