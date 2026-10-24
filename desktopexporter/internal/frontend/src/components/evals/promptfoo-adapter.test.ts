import { describe, expect, it } from 'vitest'
import { buildEvalOutline } from './eval-outline'
import { groupResults } from './group-results'
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

it('preserves selected-experiment identities with global source ordinals and local copy pointers', () => {
  const rows = [
    promptfooRow({ id: 'first' }),
    promptfooRow({ provider: { id: 'other' } }),
    promptfooRow({ id: 'third', repeatIndex: 1 }),
  ]
  const all = supported(promptfoo(rows))
  const selected = supported(promptfoo([rows[0], rows[2]]), {
    rowOrdinals: [0, 2],
  })
  expect(selected.experiments).toHaveLength(1)
  expect(selected.experiments[0].id).toBe(all.experiments[0].id)
  expect(selected.experiments[0].results.map(row => row.id)).toEqual(
    all.experiments[0].results.map(row => row.id)
  )
  expect(selected.rows[1].pointer).toBe('/results/results/1')
  const missingIdentity = promptfooRow({
    provider: undefined,
    promptId: undefined,
    promptIdx: undefined,
  })
  const full = supported(promptfoo([rows[0], missingIdentity]))
  const single = supported(promptfoo([missingIdentity]), { rowOrdinals: [1] })
  expect(single.experiments[0].id).toBe(full.experiments[1].id)
  expect(single.experiments[0].results[0].id).toBe(
    full.experiments[1].results[0].id
  )
  expect(
    projectPromptfooDataset(
      datasetSource(promptfoo(rows), { rowOrdinals: [0, 0, 2] })
    ).status
  ).toBe('error')
})

describe('persisted source dataset projections', () => {
  it.each([
    ['-0', 'number'],
    ['[-0,0,{"value":-0}]', 'array'],
    ['{"value":-0,"literal":"-0"}', 'map'],
  ])('retains signed zero in native output %s', (raw, type) => {
    const envelope = promptfoo([
      promptfooRow({ response: { output: 'OUTPUT_MARKER' } }),
    ])
    const payload = JSON.stringify(envelope).replace('"OUTPUT_MARKER"', raw)
    const projection = supported(envelope, { payload })
    expect(projection.experiments[0].results[0].answer).toBe(raw)
    expect(projection.experiments[0].results[0].answerType).toBe(type)
  })

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
    const experiment = result.experiments[0]
    expect(new Set(experiment.results.map(result => result.caseID)).size).toBe(
      1
    )
    expect(experiment.results.map(result => result.sourceID)).toEqual([
      'native-result-0',
      'native-result-1',
      'native-result-2',
    ])
    expect(
      experiment.results.every(result => result.repetition === undefined)
    ).toBe(true)
    const outline = buildEvalOutline(
      groupResults(experiment.results, experiment.results),
      experiment,
      indexEvalTraces([])
    )
    expect(outline.map(row => row.kind)).toEqual([
      'experiment',
      'case',
      'result',
      'result',
      'result',
    ])
    expect(
      outline.filter(row => row.kind === 'result').map(row => row.name)
    ).toEqual(['Result', 'Result', 'Result'])
    expect(experiment.results[1].assessments[1]).toMatchObject({
      pass: false,
      value: 0,
      explanation: 'Native explanation',
      rubric: 'Approved rubric',
    })
    expect(experiment.results[1].toolsKnown).toBe(false)
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
    const experiment = result.experiments[0]
    const outline = buildEvalOutline(
      groupResults(experiment.results, experiment.results),
      experiment,
      indexEvalTraces([])
    )
    expect(outline.map(row => row.kind)).toEqual(['experiment', 'case', 'case'])
    expect(outline[1].result?.answer).toBe('Non-chat output')
    expect(outline[1].result?.sourceID).toBe('a')
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
    expect(result.experiments).toHaveLength(1)
    const experiment = result.experiments[0]
    expect(experiment.name).toBe('Source model name')
    expect(experiment.source).toBe('User source')
    expect(experiment.configuration).toContainEqual({
      name: 'Source evaluation ID',
      value: 'eval-source-id',
    })
    expect(experiment.results[0]).toMatchObject({
      name: 'Source test name',
      answer: 'Non-chat output',
      question: 'Non-chat input',
      repetition: 1,
    })
    expect(result.rows[0]).toMatchObject({
      sourceResultID: 'source-result-id',
      evaluationID: 'eval-source-id',
      providerID: 'provider:model',
      promptID: 'source-prompt-id',
      testIndex: 0,
      repeatIndex: 0,
      pointer: '/results/results/0',
    })
    expect(result.experiments[0].results[0].id).toContain('source-result-id')
  })

  it('qualifies every presentation identity by dataset, with collision-safe components', () => {
    const a = supported(promptfoo(), { id: 'a/b' })
    const b = supported(promptfoo(), { id: 'a' })
    expect(a.experiments[0].id).not.toBe(b.experiments[0].id)
    expect(a.experiments[0].results[0].id).not.toBe(
      b.experiments[0].results[0].id
    )
    expect(a.experiments[0].results[0].caseID).not.toBe(
      b.experiments[0].results[0].caseID
    )
    expect(a.experiments[0].assessments[0].id).not.toBe(
      b.experiments[0].assessments[0].id
    )
    expect(supported(promptfoo(), { id: '\ud800' }).experiments).toHaveLength(1)
  })

  it('groups native test definitions and separates providers and prompts', () => {
    const result = supported(
      promptfoo([
        promptfooRow(),
        promptfooRow({ id: 'result-2', repeatIndex: 1 }),
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
    expect(result.experiments).toHaveLength(3)
    const results = result.experiments[0].results
    expect(results[0].caseID).toBe(results[1].caseID)
    expect(results[1].repetition).toBe(2)
    expect(results[0].caseID).not.toBe(results[2].caseID)
    expect(
      new Set(
        result.experiments.flatMap(experiment =>
          experiment.results.map(result => result.id)
        )
      ).size
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
    const results = result.experiments[0].results
    expect(results[0].caseID).toBe(results[1].caseID)
    expect(results[2].caseID).toBe(results[1].caseID)
    expect(new Set(results.map(result => result.id)).size).toBe(3)
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
      const results = result.experiments.flatMap(
        experiment => experiment.results
      )
      expect(results[0].caseID).not.toBe(results[1].caseID)
      expect(results[0].id).not.toBe(results[1].id)
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
    expect(
      result.experiments.flatMap(experiment =>
        experiment.results.map(result => result.state)
      )
    ).toEqual(['Error', 'Complete', 'Complete', 'Complete'])
    expect(result.rows.map(row => row.outcome)).toEqual([
      'execution-error',
      'assessment-failed',
      'unspecified',
      'passed',
    ])
    const results = result.experiments[0].results
    expect(results[1].assessments[0]).toMatchObject({
      label: 'Fail',
      value: 0,
      explanation: 'Latency error',
    })
    expect(results[1].error).toBeUndefined()
    expect(results[2].error).toBeUndefined()
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
      expect(result.experiments[0].results[0].assessments[0].value).toBe(
        typeof score === 'number' ? score : null
      )
      expect(result.experiments[0].assessments[0].maximum).toBeUndefined()
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
    expect(result.experiments[0].results[0].state).toBe('Complete')
    expect(result.experiments[0].results[0].assessments[0].value).toBe(0)
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
    expect(result.experiments[0].results[0]).toMatchObject({
      state: 'Error',
      error: 'Malformed grader response',
    })
    expect(result.experiments[0].results[0].assessments[0].value).toBe(0)
  })

  it.each([
    [0, 'number'],
    [false, 'boolean'],
    [null, 'null'],
    [[1, 'two'], 'array'],
    [{ answer: true }, 'map'],
    ['123', 'string'],
    ['{"answer":true}', 'string'],
  ])('retains the native output type for %j', (output, answerType) => {
    const result = supported(
      promptfoo([promptfooRow({ response: { output } })])
    )
    expect(result.experiments[0].results[0]).toMatchObject({
      answerType,
      answer: typeof output === 'string' ? output : JSON.stringify(output),
    })
    expect(result.experiments[0].configuration).toContainEqual({
      name: 'Prompt index',
      value: 0,
    })
  })

  it('does not present echo replay latency as model execution duration', () => {
    for (const latencyMs of [0, 1, 250]) {
      const result = supported(
        promptfoo([promptfooRow({ provider: { id: 'echo' }, latencyMs })])
      )
      expect(result.experiments[0].results[0].seconds).toBeUndefined()
      const execution = supported(promptfoo([promptfooRow({ latencyMs })]))
      expect(execution.experiments[0].results[0].seconds).toBe(latencyMs / 1000)
    }
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
      expect(result.experiments[0].results[0]).toMatchObject({
        seconds: 0,
        answer: '',
      })
      expect(result.experiments[0].results[0].investigatorCost).toBeUndefined()
      expect(result.experiments[0].results[0].graderCost).toBeUndefined()
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
    expect(native.experiments[0].results[0].telemetry).toEqual({
      traceID,
      spanID: undefined,
    })
    expect(native.rows[0].evaluationID).toBe('trace-evaluation')
    expect(native.experiments[0].results[0].tools).toEqual([])
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
    expect(prose.experiments[0].results[0].telemetry).toBeUndefined()
    expect(prose.experiments[0].results[0].tools).toEqual([])
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
    const results = result.experiments[0].results
    expect(results.slice(0, 4).map(result => result.answer)).toEqual([
      undefined,
      'null',
      'false',
      '0',
    ])
    expect(results[4].answer).toContain('file:///opaque')
    expect(results[5].answer!.length).toBeLessThan(33000)
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
      result.experiments[0].results.reduce(
        (sum, result) => sum + (result.answer?.length ?? 0),
        0
      )
    ).toBeLessThan(evalProjectionLimits.projectedCharacters + 1000)
  })
})
