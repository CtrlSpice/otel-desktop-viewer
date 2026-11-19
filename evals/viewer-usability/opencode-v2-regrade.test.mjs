import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { tmpdir } from 'node:os'
import { createHash } from 'node:crypto'
import { GPT_MODELS, OPUS_MODEL, selectGPTResponses, savedGradingInput, comparisonExport, gradeSavedInput, loadCompletedSource } from './opencode-v2-regrade.mjs'
import { v2GradingPrompt } from './opencode-v2-grading-evidence.mjs'

const hash = value => createHash('sha256').update(value).digest('hex')
const write = (file, value) => fs.writeFileSync(file, JSON.stringify(value))
const plan = { mode: 'full', repeat: 3, taskIDs: Array.from({ length: 11 }, (_, index) => 'question-' + index) }
const rows = () => GPT_MODELS.flatMap(model => plan.taskIDs.flatMap(task => Array.from({ length: 3 }, (_, repeatIndex) => ({
  id: `${model}/${task}/${repeatIndex}`, vars: { task_id: task }, response: { output: 'saved answer',
    metadata: { runtime: 'opencode-v2', role: 'investigator', model, task, repeatIndex } },
  success: false, score: 0, failureReason: 1, error: 'Original judgment',
}))))

test('selection is exactly the original 66 GPT slots, without altering the source', () => {
  const original = { results: { results: [...rows(), { response: { metadata: { model: 'anthropic/claude-opus-5-5' } } }] } }
  const before = JSON.stringify(original)
  assert.equal(selectGPTResponses(original, plan).length, 66)
  assert.equal(JSON.stringify(original), before)
  assert.throws(() => selectGPTResponses({ results: { results: rows().slice(1) } }, plan), /all planned GPT/)
  const duplicate = rows(); duplicate[1] = duplicate[0]
  assert.throws(() => selectGPTResponses({ results: { results: duplicate } }, plan), /Repeated original/)
})

test('an active original run cannot be graded or mutated', t => {
  const root = fs.mkdtempSync(path.join(tmpdir(), 'opencode/active-grading-source-'))
  t.after(() => fs.rmSync(root, { recursive: true, force: true }))
  write(path.join(root, 'sentinel.json'), { original: true })
  assert.throws(() => loadCompletedSource(root), /original run is still running/)
  assert.deepEqual(fs.readdirSync(root), ['sentinel.json'])
})

test('saved rubric, answer and tool records must match their received hashes', t => {
  const root = fs.realpathSync(fs.mkdtempSync(path.join(tmpdir(), 'opencode/saved-grading-input-')))
  t.after(() => fs.rmSync(root, { recursive: true, force: true }))
  const evidence = path.join(root, 'runs/investigator-fixture')
  fs.mkdirSync(evidence, { recursive: true })
  const row = rows()[0]
  row.response.metadata = { ...row.response.metadata, evidence, sessionID: 'ses_fixture' }
  write(path.join(evidence, 'response.json'), row.response)
  write(path.join(evidence, 'identity.json'), { promptfoo: { evaluationId: 'original', repeatIndex: 0 } })
  write(path.join(evidence, 'context.json'), { data: [] })
  const tools = [{ id: 'failed-call', sessionID: 'ses_fixture', state: { status: 'error', error: 'recorded tool failure' } }]
  write(path.join(evidence, 'tools.json'), tools)
  const packet = { question: 'original question', reference: 'original reference', criteria: { pass: 'original criterion' },
    evidence: { investigator: { evidence, model: row.response.metadata.model, taskID: row.vars.task_id },
      answerSHA256: hash(row.response.output), events: tools.map(tool => ({ raw: JSON.stringify(tool) })),
      sources: Object.fromEntries(['context.json', 'tools.json'].map(name => [name, hash(fs.readFileSync(path.join(evidence, name)))])) } }
  write(path.join(evidence, 'grading-packet.json'), packet)
  row.gradingResult = { componentResults: [{ assertion: { type: 'llm-rubric', threshold: 1 },
    metadata: { renderedAssertionValue: v2GradingPrompt(packet) } }] }
  const input = savedGradingInput(root, 'original', row)
  assert.equal(input.criterion, v2GradingPrompt(packet))
  assert.ok(input.criterion.includes('recorded tool failure'))
  write(path.join(evidence, 'tools.json'), [])
  assert.throws(() => savedGradingInput(root, 'original', row))
})

test('comparison retains original answers and IDs without copying stale errors or scores', () => {
  const selected = rows()
  const before = JSON.stringify(selected)
  const grade = { pass: true, score: 1, reason: 'Opus judgment', componentResults: [{ assertion: { type: 'llm-rubric' }, pass: true, score: 1 }] }
  const judgments = new Map(selected.map(row => [row.id, grade]))
  const report = comparisonExport({ evalId: 'original' }, selected, judgments, 'comparison', 'now')
  assert.equal(report.config.metadata.derivedComparison, true)
  assert.equal(report.config.metadata.grader, `claude-code:${OPUS_MODEL}`)
  for (const [index, row] of report.results.results.entries()) {
    assert.equal(row.id, selected[index].id)
    assert.deepEqual(row.response, selected[index].response)
    assert.equal(row.score, 1)
    assert.equal(Object.hasOwn(row, 'error'), false)
  }
  assert.deepEqual(JSON.parse(JSON.stringify(report)), report)
  assert.equal(JSON.stringify(selected), before)
  assert.throws(() => comparisonExport({ evalId: 'original' }, selected, new Map(), 'comparison', 'now'), /must have/)
})

test('native assertions perform one fresh grade and reuse the recorded artifact result', async t => {
  const root = fs.mkdtempSync(path.join(tmpdir(), 'opencode/native-opus-grading-'))
  t.after(() => fs.rmSync(root, { recursive: true, force: true }))
  process.env.PROMPTFOO_DISABLE_TELEMETRY = '1'
  process.env.PROMPTFOO_CONFIG_DIR = path.join(root, 'state')
  const { assertions, cache } = await import('promptfoo')
  cache.disableCache()
  for (const verdict of ['partial', 'malformed']) {
    let calls = 0
    const provider = { id: () => 'synthetic-opus-grader', callApi: async prompt => {
      calls++
      assert.ok(prompt.includes('saved answer'))
      assert.ok(prompt.includes('approved saved evidence'))
      return { output: verdict === 'malformed' ? 'invalid verdict' : '{"pass":true,"score":0.5,"reason":"Partial evidence"}' }
    } }
    const row = rows()[0]
    const originalCheck = { assertion: { type: 'javascript', metric: 'export-file' }, pass: true, score: 1, reason: 'Original artifact verified' }
    const input = { row, criterion: 'approved saved evidence',
      components: [{ assertion: { type: 'llm-rubric', threshold: 1 } }, originalCheck] }
    const grade = await gradeSavedInput(input, provider, assertions.runAssertions)
    assert.equal(calls, 1)
    assert.equal(grade.pass, false)
    assert.deepEqual(grade.componentResults[1].assertion, originalCheck.assertion)
    assert.equal(grade.componentResults[1].metadata.reusedOriginalAssertion, true)
    if (verdict === 'partial') assert.equal(grade.componentResults[0].score, 0.5)
    else assert.equal(grade.componentResults[0].metadata.graderError, true)
  }
})

for (const criterion of ['literal recorded tool {{ task_id }} end', 'literal recorded tool {% badtag %} end'])
  test(`native grading keeps saved delimiters literal: ${criterion}`, async t => {
    const root = fs.mkdtempSync(path.join(tmpdir(), 'opencode/literal-opus-grading-'))
    t.after(() => fs.rmSync(root, { recursive: true, force: true }))
    process.env.PROMPTFOO_DISABLE_TELEMETRY = '1'
    process.env.PROMPTFOO_CONFIG_DIR = path.join(root, 'state')
    const { assertions, cache } = await import('promptfoo')
    cache.disableCache()
    const row = rows()[0]
    row.vars.task_id = 'do-not-substitute'
    const component = { assertion: { type: 'llm-rubric', threshold: 1 } }
    let expectedPrompt, receivedPrompt, calls = 0
    const response = { output: '{"pass":true,"score":1,"reason":"Recorded evidence"}' }
    const expectedProvider = { id: () => 'synthetic-opus-grader', callApi: async prompt => { expectedPrompt = prompt; return response } }
    await assertions.matchesLlmRubric(criterion, row.response.output, { provider: expectedProvider },
      row.vars, component.assertion)
    const actualProvider = { id: () => 'synthetic-opus-grader', callApi: async prompt => { calls++; receivedPrompt = prompt; return response } }
    const result = await gradeSavedInput({ row, criterion, components: [component] }, actualProvider, assertions.runAssertions)
    assert.equal(calls, 1)
    assert.equal(receivedPrompt, expectedPrompt, 'Opus must receive the exact native prompt for the unmodified criterion')
    assert.ok(JSON.parse(receivedPrompt).some(message => message.content.includes(criterion)))
    assert.equal(result.componentResults[0].metadata.renderedAssertionValue, criterion)
    assert.equal(result.pass, true)
  })
