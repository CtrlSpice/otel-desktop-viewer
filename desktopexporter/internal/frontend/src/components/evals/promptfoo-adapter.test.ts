import { describe, expect, it } from 'vitest'
import { buildEvalOutline } from './eval-outline'
import { groupReps } from './group-reps'
import { indexEvalTraces } from './eval-traces'
import {
  projectPromptfooDataset,
  evalProjectionLimits,
  type EvalSourceDataset,
} from './promptfoo-adapter'

// Synthetic fixtures following the pinned Promptfoo sample-export.json.
function promptfoo(rows: Record<string, unknown>[] = [promptfooRow()]) {
  return {
    evalId: 'eval-source-id',
    config: { description: 'Recorded evaluation', extra: { native: true } },
    results: {
      version: 3,
      timestamp: '2026-10-16T21:30:05.123Z',
      results: rows,
    },
  }
}

function promptfooRow(overrides: Record<string, unknown> = {}) {
  return {
    id: 'source-result-id',
    provider: { id: 'provider:model', label: 'Source model name' },
    promptId: 'source-prompt-id',
    promptIdx: 0,
    prompt: { raw: 'Non-chat input', label: 'Source prompt name' },
    testIdx: 0,
    repeatIndex: 0,
    testCase: { description: 'Source test name' },
    response: { output: 'Non-chat output' },
    success: true,
    score: 1,
    failureReason: 0,
    ...overrides,
  }
}

function datasetSource(
  value: unknown,
  overrides: Partial<EvalSourceDataset> = {}
): EvalSourceDataset {
  return {
    id: 'dataset-a',
    name: 'Saved evaluation',
    source: 'User source',
    format: 'json',
    payload: JSON.stringify(value),
    ...overrides,
  }
}

function supported(value: unknown, overrides: Partial<EvalSourceDataset> = {}) {
  const result = projectPromptfooDataset(datasetSource(value, overrides))
  expect(result.status).toBe('supported')
  if (result.status !== 'supported') throw new Error(result.reason)
  return result
}

it('preserves selected-run identities with global source ordinals and local copy pointers', () => {
  const rows = [
    promptfooRow({ id: 'first' }),
    promptfooRow({ provider: { id: 'other' } }),
    promptfooRow({ id: 'third', repeatIndex: 1 }),
  ]
  const all = supported(promptfoo(rows))
  const selected = supported(promptfoo([rows[0], rows[2]]), {
    rowOrdinals: [0, 2],
  })
  expect(selected.runs).toHaveLength(1)
  expect(selected.runs[0].id).toBe(all.runs[0].id)
  expect(selected.runs[0].reps.map(row => row.id)).toEqual(
    all.runs[0].reps.map(row => row.id)
  )
  expect(selected.rows[1].pointer).toBe('/results/results/1')
  const missingIdentity = promptfooRow({
    provider: undefined,
    promptId: undefined,
    promptIdx: undefined,
  })
  const full = supported(promptfoo([rows[0], missingIdentity]))
  const single = supported(promptfoo([missingIdentity]), { rowOrdinals: [1] })
  expect(single.runs[0].id).toBe(full.runs[1].id)
  expect(single.runs[0].reps[0].id).toBe(full.runs[1].reps[0].id)
  expect(
    projectPromptfooDataset(
      datasetSource(promptfoo(rows), { rowOrdinals: [0, 0, 2] })
    ).status
  ).toBe('error')
})

describe('persisted source dataset projections', () => {
  it('groups native expanded test indices without custom repetition metadata or invented phase rows', () => {
    const result = supported(
      promptfoo(
        [0, 1, 2].map(testIdx =>
          promptfooRow({
            id: `native-result-${testIdx}`,
            testIdx,
            repeatIndex: undefined,
            latencyMs: 0,
            gradingResult: {
              pass: testIdx !== 1,
              score: testIdx === 1 ? 0 : 1,
              componentResults: [
                {
                  pass: testIdx !== 1,
                  score: testIdx === 1 ? 0 : 1,
                  reason: 'Native explanation',
                  assertion: { type: 'llm-rubric', value: 'Approved rubric' },
                },
              ],
            },
            success: testIdx !== 1,
            failureReason: testIdx === 1 ? 1 : 0,
          })
        )
      )
    )
    const run = result.runs[0]
    expect(new Set(run.reps.map(rep => rep.caseID)).size).toBe(1)
    expect(run.reps.map(rep => rep.sourceID)).toEqual([
      'native-result-0',
      'native-result-1',
      'native-result-2',
    ])
    expect(run.reps.every(rep => rep.repetition === undefined)).toBe(true)
    const outline = buildEvalOutline(
      groupReps(run.reps, run.reps),
      run,
      indexEvalTraces([])
    )
    expect(outline.map(row => row.kind)).toEqual([
      'run',
      'case',
      'rep',
      'rep',
      'rep',
    ])
    expect(
      outline.filter(row => row.kind === 'rep').map(row => row.name)
    ).toEqual(['Result', 'Result', 'Result'])
    expect(run.reps[1].assessments[1]).toMatchObject({
      pass: false,
      value: 0,
      explanation: 'Native explanation',
      rubric: 'Approved rubric',
    })
    expect(run.reps[1].toolsKnown).toBe(false)
  })

  it('puts a singleton result directly on its test case and keeps resolved inputs distinct', () => {
    const result = supported(
      promptfoo([
        promptfooRow({
          id: 'a',
          repeatIndex: undefined,
          vars: { input: 'one' },
        }),
        promptfooRow({
          id: 'b',
          repeatIndex: undefined,
          vars: { input: 'two' },
        }),
      ])
    )
    const run = result.runs[0]
    const outline = buildEvalOutline(
      groupReps(run.reps, run.reps),
      run,
      indexEvalTraces([])
    )
    expect(outline.map(row => row.kind)).toEqual(['run', 'case', 'case'])
    expect(outline[1].rep?.answer).toBe('Non-chat output')
    expect(outline[1].rep?.sourceID).toBe('a')
  })
  it('accepts Promptfoo declarations and rejects unsupported formats', () => {
    expect(supported(promptfoo(), { format: 'promptfoo-v3' }).adapter).toBe(
      'promptfoo-v3'
    )
    expect(
      projectPromptfooDataset(
        datasetSource(promptfoo(), { format: 'unknown-json' })
      ).status
    ).toBe('unsupported')
    expect(
      projectPromptfooDataset(datasetSource(promptfoo(), { format: 'unknown' }))
    ).toMatchObject({
      status: 'unsupported',
      reason: 'This declared format has no display adapter.',
    })
  })
  it('projects a full v3 export with native names and source references', () => {
    const result = supported(promptfoo())
    expect(result.adapter).toBe('promptfoo-v3')
    expect(result.runs).toHaveLength(1)
    const run = result.runs[0]
    expect(run.name).toBe('Source model name')
    expect(run.source).toBe('User source')
    expect(run.configuration).toContainEqual({
      name: 'Source evaluation ID',
      value: 'eval-source-id',
    })
    expect(run.reps[0]).toMatchObject({
      name: 'Source test name',
      answer: 'Non-chat output',
      question: 'Non-chat input',
      repetition: 1,
    })
    expect(result.rows[0]).toMatchObject({
      resultID: 'source-result-id',
      evaluationID: 'eval-source-id',
      providerID: 'provider:model',
      promptID: 'source-prompt-id',
      testIndex: 0,
      repeatIndex: 0,
      pointer: '/results/results/0',
    })
    expect(result.runs[0].reps[0].id).toContain('source-result-id')
  })

  it('qualifies every presentation identity by dataset, with collision-safe components', () => {
    const a = supported(promptfoo(), { id: 'a/b' })
    const b = supported(promptfoo(), { id: 'a' })
    expect(a.runs[0].id).not.toBe(b.runs[0].id)
    expect(a.runs[0].reps[0].id).not.toBe(b.runs[0].reps[0].id)
    expect(a.runs[0].reps[0].caseID).not.toBe(b.runs[0].reps[0].caseID)
    expect(a.runs[0].assessments[0].id).not.toBe(b.runs[0].assessments[0].id)
    expect(supported(promptfoo(), { id: '\ud800' }).runs).toHaveLength(1)
  })

  it('groups native test definitions and separates providers and prompts', () => {
    const result = supported(
      promptfoo([
        promptfooRow(),
        promptfooRow({ id: 'rep-2', repeatIndex: 1 }),
        promptfooRow({
          id: 'other-test',
          testIdx: 1,
          testCase: { description: 'Another test' },
        }),
        promptfooRow({
          id: 'other-provider',
          provider: { id: 'other:model', label: 'Source model name' },
        }),
        promptfooRow({ id: 'other-prompt', promptId: 'other-prompt' }),
      ])
    )
    expect(result.runs).toHaveLength(3)
    const reps = result.runs[0].reps
    expect(reps[0].caseID).toBe(reps[1].caseID)
    expect(reps[1].repetition).toBe(2)
    expect(reps[0].caseID).not.toBe(reps[2].caseID)
    expect(
      new Set(result.runs.flatMap(run => run.reps.map(rep => rep.id))).size
    ).toBe(5)
  })

  it('uses repeatGroupId across expanded test indices and keeps each result distinct', () => {
    const result = supported(
      promptfoo([
        promptfooRow({ repeatGroupId: 'native-group' }),
        promptfooRow({
          repeatGroupId: 'native-group',
          repeatIndex: 1,
          testIdx: 10,
        }),
        promptfooRow({
          repeatGroupId: 'native-group',
          repeatIndex: 1,
          testIdx: 10,
        }),
      ])
    )
    const reps = result.runs[0].reps
    expect(reps[0].caseID).toBe(reps[1].caseID)
    expect(reps[2].caseID).toBe(reps[1].caseID)
    expect(new Set(reps.map(rep => rep.id)).size).toBe(3)
  })

  it('never merges equal question text when required identities are missing', () => {
    for (const missing of ['testCase', 'provider', 'promptId']) {
      const overrides = {
        [missing]: undefined,
        testIdx: undefined,
        promptIdx: missing === 'promptId' ? undefined : 0,
      }
      const row = promptfooRow(overrides)
      const result = supported(promptfoo([row, row]))
      const reps = result.runs.flatMap(run => run.reps)
      expect(reps[0].caseID).not.toBe(reps[1].caseID)
      expect(reps[0].id).not.toBe(reps[1].id)
    }
  })

  it('uses only failureReason 2 for execution error, never error prose or zero scores', () => {
    const result = supported(
      promptfoo([
        promptfooRow({ failureReason: 2, error: 'Transport failed', score: 0 }),
        promptfooRow({
          failureReason: 1,
          error: 'Latency error',
          score: 0,
          success: false,
        }),
        promptfooRow({
          failureReason: undefined,
          error: 'Fatal execution error',
          score: 0,
          success: undefined,
          response: { error: 'Request failed' },
        }),
        promptfooRow({ failureReason: 0, error: 'error', score: 0 }),
      ])
    )
    expect(result.runs.flatMap(run => run.reps.map(rep => rep.state))).toEqual([
      'Error',
      'Complete',
      'Complete',
      'Complete',
    ])
    expect(result.rows.map(row => row.outcome)).toEqual([
      'execution-error',
      'assessment-failed',
      'unspecified',
      'passed',
    ])
    const reps = result.runs[0].reps
    expect(reps[1].assessments[0]).toMatchObject({
      label: 'Fail',
      value: 0,
      explanation: 'Latency error',
    })
    expect(reps[1].error).toBeUndefined()
    expect(reps[2].error).toBeUndefined()
  })

  it('does not infer numeric scales or reducers from arbitrary or named Promptfoo scores', () => {
    for (const score of [0, 0.5, 1, 7, -4, 'A', { quality: 3 }]) {
      const result = supported(
        promptfoo([
          promptfooRow({
            score,
            gradingResult: { score },
            namedScores: { quality: 42 },
          }),
        ])
      )
      expect(result.runs[0].reps[0].assessments[0].value).toBe(
        typeof score === 'number' ? score : null
      )
      expect(result.runs[0].assessments[0].maximum).toBeUndefined()
      expect(result.notes.join(' ')).toContain('no guaranteed maximum')
    }
  })

  it('does not treat success false without a failure discriminator as an assessment failure', () => {
    const result = supported(
      promptfoo([
        promptfooRow({ failureReason: undefined, success: false, score: 0 }),
      ])
    )
    expect(result.rows[0].outcome).toBe('unspecified')
    expect(result.runs[0].reps[0].state).toBe('Complete')
    expect(result.runs[0].reps[0].assessments[0].value).toBe(0)
  })

  it('distinguishes native grader errors from an assessed failure without changing the source score', () => {
    const result = supported(
      promptfoo([
        promptfooRow({
          score: 0,
          success: false,
          failureReason: 1,
          gradingResult: {
            reason: 'Malformed grader response',
            componentResults: [
              {
                pass: false,
                score: 0,
                metadata: { graderError: true },
                assertion: { type: 'llm-rubric' },
              },
            ],
          },
        }),
      ])
    )
    expect(result.rows[0].outcome).toBe('execution-error')
    expect(result.runs[0].reps[0]).toMatchObject({
      state: 'Error',
      error: 'Malformed grader response',
    })
    expect(result.runs[0].reps[0].assessments[0].value).toBe(0)
  })

  it('keeps missing usage and incompatible zero/nonzero costs unknown', () => {
    for (const value of [undefined, 0, 0.25]) {
      const result = supported(
        promptfoo([
          promptfooRow({
            cost: value,
            currency: 'USD',
            latencyMs: 0,
            response: { cost: value, output: '' },
          }),
        ])
      )
      expect(result.runs[0].reps[0]).toMatchObject({
        seconds: 0,
        answer: '',
      })
      expect(result.runs[0].reps[0].investigatorCost).toBeUndefined()
      expect(result.runs[0].reps[0].graderCost).toBeUndefined()
      expect(result.notes.join(' ')).toContain('UI labels costs CAD')
    }
  })

  it('preserves original unknown fields, numeric lexemes and whitespace by caller contract', () => {
    const payload =
      '{ "evalId":"native", "config":{}, "results":{"version":3,"results":[]}, "unknown":{"exact":1.00,"signed":"opaque","nested":[true,null]}}\n'
    const source = Object.freeze(datasetSource(null, { payload }))
    const result = projectPromptfooDataset(source)
    expect(result.status).toBe('supported')
    expect(source.payload).toBe(payload)
    expect(result).not.toHaveProperty('payload')
    expect(JSON.parse(source.payload).unknown).toEqual({
      exact: 1,
      signed: 'opaque',
      nested: [true, null],
    })
  })

  it('links only schema-established Promptfoo telemetry', () => {
    const traceID = 'a'.repeat(32)
    const spanID = 'b'.repeat(16)
    const native = supported(
      promptfoo([
        promptfooRow({ traceId: traceID, evaluationId: 'trace-evaluation' }),
      ])
    )
    expect(native.runs[0].reps[0].telemetry).toEqual({
      traceID,
      spanID: undefined,
    })
    expect(native.rows[0].evaluationID).toBe('trace-evaluation')
    expect(native.runs[0].reps[0].tools).toEqual([])
    const prose = supported(
      promptfoo([
        promptfooRow({
          response: {
            output: `Tool call bash traceID=${traceID}`,
            metadata: { telemetry: { traceID, spanID } },
          },
        }),
      ])
    )
    expect(prose.runs[0].reps[0].telemetry).toBeUndefined()
    expect(prose.runs[0].reps[0].tools).toEqual([])
  })

  it('handles missing, structured, scalar and long outputs without chat assumptions', () => {
    const outputs = [
      undefined,
      null,
      false,
      0,
      { content: [{ type: 'image', url: 'file:///opaque' }] },
      'x'.repeat(100000),
    ]
    const result = supported(
      promptfoo(outputs.map(output => promptfooRow({ response: { output } })))
    )
    const reps = result.runs[0].reps
    expect(reps.slice(0, 4).map(rep => rep.answer)).toEqual([
      undefined,
      'null',
      'false',
      '0',
    ])
    expect(reps[4].answer).toContain('file:///opaque')
    expect(reps[5].answer!.length).toBeLessThan(33000)
    expect(result.notes.join(' ')).toContain('Long display text is truncated')
  })

  it('rejects malformed envelopes and selected row discriminators explicitly', () => {
    for (const value of [
      { evalId: 'x', results: { version: 3, results: [] } },
      { ...promptfoo(), results: { version: 3, results: {} } },
      promptfoo([null as never]),
      promptfoo([promptfooRow({ repeatIndex: -1 })]),
      promptfoo([promptfooRow({ failureReason: '2' })]),
    ]) {
      expect(projectPromptfooDataset(datasetSource(value)).status).toBe('error')
    }
    expect(
      projectPromptfooDataset(datasetSource(null, { payload: '{' })).status
    ).toBe('error')
    for (const value of [
      [],
      { custom: 'native' },
      { manifest: {}, pilot: {} },
      { ...promptfoo(), results: { version: 2, results: [] } },
    ]) {
      expect(projectPromptfooDataset(datasetSource(value)).status).toBe(
        'unsupported'
      )
    }
    expect(
      projectPromptfooDataset(datasetSource(promptfoo(), { format: 'jsonl' }))
        .status
    ).toBe('unsupported')
    expect(supported(promptfoo(), { format: 'application/json' }).adapter).toBe(
      'promptfoo-v3'
    )
  })

  it('rejects unsafe/nonfinite numbers even in unknown fields and retains original text', () => {
    for (const lexeme of ['9007199254740993', '1e400', '-9007199254740993']) {
      const source = datasetSource(null, {
        payload: `{"evalId":"x","config":{},"results":{"version":3,"results":[]},"unknown":${lexeme}}`,
      })
      const original = source.payload
      const result = projectPromptfooDataset(source)
      expect(result.status).toBe('error')
      if (result.status !== 'supported')
        expect(result.reason).toContain('unsafe integer')
      expect(source.payload).toBe(original)
    }
  })

  it('bounds payload, rows, nesting, node count and total display text', () => {
    expect(
      projectPromptfooDataset(
        datasetSource(null, {
          payload: ' '.repeat(evalProjectionLimits.payloadCharacters + 1),
        })
      ).status
    ).toBe('error')
    expect(
      projectPromptfooDataset(
        datasetSource(
          promptfoo(
            Array.from({ length: evalProjectionLimits.rows + 1 }, () => ({}))
          )
        )
      ).status
    ).toBe('error')
    expect(
      projectPromptfooDataset(
        datasetSource(null, {
          payload: '['.repeat(100) + '0' + ']'.repeat(100),
        })
      ).status
    ).toBe('error')
    expect(
      projectPromptfooDataset(
        datasetSource({
          unknown: Array(evalProjectionLimits.nodes + 1).fill(0),
        })
      ).status
    ).toBe('error')
    expect(
      projectPromptfooDataset(
        datasetSource({
          ...promptfoo(),
          evalId: 'x'.repeat(evalProjectionLimits.identityCharacters + 1),
        })
      ).status
    ).toBe('error')
    const result = supported(
      promptfoo(
        Array.from({ length: 40 }, (_, repeatIndex) =>
          promptfooRow({ repeatIndex, response: { output: 'x'.repeat(32768) } })
        )
      )
    )
    expect(
      result.runs[0].reps.reduce(
        (sum, rep) => sum + (rep.answer?.length ?? 0),
        0
      )
    ).toBeLessThan(evalProjectionLimits.projectedCharacters + 1000)
  })
})
