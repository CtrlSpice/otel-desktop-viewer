import { describe, expect, it } from 'vitest'
import {
  projectEvalSnapshot,
  snapshotProjectionLimits,
  type EvalSourceSnapshot,
} from './snapshot-adapters'

// Synthetic fixtures following the pinned Promptfoo sample-export.json and the
// kit native-evaluate.ts/grade.ts/eval-telemetry.ts shapes. No saved user outputs.
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

function kitRow(overrides: Record<string, unknown> = {}) {
  return promptfooRow({
    vars: {
      task_id: 'trace-export',
      goal: 'Export the trace',
      eval_model: 'test/model',
    },
    response: {
      output: 'Recorded answer',
      metadata: { model: 'test/model', elapsedSeconds: 0 },
    },
    gradingResult: { pass: false, score: 0.5, reason: 'Recorded explanation' },
    ...overrides,
  })
}

function kit(rows = [kitRow()]) {
  return {
    manifest: {
      models: ['test/model'],
      tasks: 11,
      repetitions: 3,
      startedAt: '2026-10-16T21:30:05.123Z',
    },
    pilot: { results: { results: rows } },
    modelSummary: { models: [{ model: 'test/model', apiCostCad: 7 }] },
    reviewHistory: [{ signedScore: 1, signature: 'opaque-unverified' }],
    deferredSlots: [{ task: 'trace-export', repetition: 2 }],
    retryOf: 'previous-snapshot',
  }
}

function snapshot(
  value: unknown,
  overrides: Partial<EvalSourceSnapshot> = {}
): EvalSourceSnapshot {
  return {
    id: 'snapshot-a',
    name: 'Saved evaluation',
    source: 'User source',
    format: 'json',
    payload: JSON.stringify(value),
    ...overrides,
  }
}

function supported(
  value: unknown,
  overrides: Partial<EvalSourceSnapshot> = {}
) {
  const result = projectEvalSnapshot(snapshot(value, overrides))
  expect(result.status).toBe('supported')
  if (result.status !== 'supported') throw new Error(result.reason)
  return result
}

describe('persisted source snapshot projections', () => {
  it('honors archive format declarations and rejects mismatched producer envelopes', () => {
    expect(supported(promptfoo(), { format: 'promptfoo-v3' }).adapter).toBe(
      'promptfoo-v3'
    )
    expect(supported(kit(), { format: 'viewer-kit' }).adapter).toBe(
      'viewer-kit'
    )
    expect(
      projectEvalSnapshot(snapshot(promptfoo(), { format: 'unknown-json' }))
        .status
    ).toBe('unsupported')
    expect(
      projectEvalSnapshot(snapshot(promptfoo(), { format: 'viewer-kit' }))
    ).toMatchObject({
      status: 'unsupported',
      reason: 'The JSON envelope does not match the declared source format.',
    })
    expect(
      projectEvalSnapshot(snapshot(kit(), { format: 'promptfoo-v3' }))
    ).toMatchObject({
      status: 'unsupported',
      reason: 'The JSON envelope does not match the declared source format.',
    })
  })
  it('projects a full v3 export with native names and source references', () => {
    const result = supported(promptfoo())
    expect(result.adapter).toBe('promptfoo-v3')
    expect(result.runs).toHaveLength(1)
    const run = result.runs[0]
    expect(run.name).toBe(
      'Recorded evaluation · Source model name · Source prompt name'
    )
    expect(run.source).toBe('User source')
    expect(run.configuration).toContainEqual({
      name: 'Source evaluation ID',
      value: 'eval-source-id',
    })
    expect(run.attempts[0]).toMatchObject({
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
    expect(result.runs[0].attempts[0].id).toContain('source-result-id')
  })

  it('qualifies every presentation identity by snapshot, with collision-safe components', () => {
    const a = supported(promptfoo(), { id: 'a/b' })
    const b = supported(promptfoo(), { id: 'a' })
    expect(a.runs[0].id).not.toBe(b.runs[0].id)
    expect(a.runs[0].attempts[0].id).not.toBe(b.runs[0].attempts[0].id)
    expect(a.runs[0].attempts[0].caseID).not.toBe(b.runs[0].attempts[0].caseID)
    expect(a.runs[0].assessments[0].id).not.toBe(b.runs[0].assessments[0].id)
    expect(supported(promptfoo(), { id: '\ud800' }).runs).toHaveLength(1)
  })

  it('groups only explicit repetition identity and separates providers and prompts', () => {
    const result = supported(
      promptfoo([
        promptfooRow(),
        promptfooRow({ id: 'rep-2', repeatIndex: 1 }),
        promptfooRow({ id: 'other-test', testIdx: 1 }),
        promptfooRow({
          id: 'other-provider',
          provider: { id: 'other:model', label: 'Source model name' },
        }),
        promptfooRow({ id: 'other-prompt', promptId: 'other-prompt' }),
      ])
    )
    expect(result.runs).toHaveLength(3)
    const attempts = result.runs[0].attempts
    expect(attempts[0].caseID).toBe(attempts[1].caseID)
    expect(attempts[1].repetition).toBe(2)
    expect(attempts[0].caseID).not.toBe(attempts[2].caseID)
    expect(
      new Set(
        result.runs.flatMap(run => run.attempts.map(attempt => attempt.id))
      ).size
    ).toBe(5)
  })

  it('uses repeatGroupId across expanded test indices and keeps duplicate slots distinct', () => {
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
    const attempts = result.runs[0].attempts
    expect(attempts[0].caseID).toBe(attempts[1].caseID)
    expect(attempts[2].caseID).not.toBe(attempts[1].caseID)
    expect(new Set(attempts.map(attempt => attempt.id)).size).toBe(3)
    expect(result.notes.join(' ')).toContain('Duplicate repetition')
  })

  it('never merges equal question text when required identities are missing', () => {
    for (const missing of ['repeatIndex', 'testIdx', 'provider', 'promptId']) {
      const overrides = {
        [missing]: undefined,
        promptIdx: missing === 'promptId' ? undefined : 0,
      }
      const row = promptfooRow(overrides)
      const result = supported(promptfoo([row, row]))
      const attempts = result.runs.flatMap(run => run.attempts)
      expect(attempts[0].caseID).not.toBe(attempts[1].caseID)
      expect(attempts[0].id).not.toBe(attempts[1].id)
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
      result.runs.flatMap(run => run.attempts.map(attempt => attempt.state))
    ).toEqual(['Error', 'Complete', 'Complete', 'Complete'])
    expect(result.rows.map(row => row.outcome)).toEqual([
      'execution-error',
      'assessment-failed',
      'unspecified',
      'passed',
    ])
    const attempts = result.runs[0].attempts
    expect(attempts[1].assessments[0]).toMatchObject({
      label: 'Fail',
      value: null,
      explanation: 'Latency error',
    })
    expect(attempts[1].error).toBeUndefined()
    expect(attempts[2].error).toBeUndefined()
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
      expect(result.runs[0].attempts[0].assessments[0].value).toBeNull()
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
    expect(result.runs[0].attempts[0].state).toBe('Complete')
    expect(result.runs[0].attempts[0].assessments[0].value).toBeNull()
  })

  it('projects the explicit kit wrapper and documented completion convention only', () => {
    const result = supported(
      kit(
        [0, 0.5, 1, 0.7, { named: 1 }].map((score, repeatIndex) =>
          kitRow({
            repeatIndex,
            gradingResult: { score, reason: 'Recorded explanation' },
          })
        )
      )
    )
    expect(result.adapter).toBe('viewer-kit')
    expect(result.rows[0]).toMatchObject({
      taskID: 'trace-export',
      pointer: '/pilot/results/results/0',
    })
    const attempts = result.runs[0].attempts
    expect(attempts.map(attempt => attempt.assessments[0].value)).toEqual([
      0,
      0.5,
      1,
      null,
      null,
    ])
    expect(attempts[0]).toMatchObject({
      question: 'Export the trace',
      seconds: 0,
    })
    expect(result.runs[0].assessments[0].aggregation).toBe('scored')
    expect(result.notes.join(' ')).toContain(
      'not the kit report planned-slot means'
    )
    expect(result.runs.flatMap(run => run.attempts)).toHaveLength(5)
  })

  it('retains recorded kit grades independently from execution failure and unverified reviews', () => {
    const result = supported(
      kit([
        kitRow({
          failureReason: 2,
          gradingResult: { score: 0 },
          response: { metadata: { harnessBlocker: true } },
        }),
      ])
    )
    expect(result.runs[0].attempts[0]).toMatchObject({
      state: 'Error',
      assessments: [{ value: 0 }],
    })
    expect(result.notes.join(' ')).toContain('Reviews, retries, deferred slots')
  })

  it('uses kit task IDs when no numeric test index was supplied', () => {
    const result = supported(
      kit([
        kitRow({ testIdx: undefined }),
        kitRow({ testIdx: undefined, repeatIndex: 1 }),
        kitRow({
          testIdx: undefined,
          vars: { task_id: 'different-task', goal: 'Export the trace' },
        }),
      ])
    )
    const attempts = result.runs[0].attempts
    expect(attempts[0].caseID).toBe(attempts[1].caseID)
    expect(attempts[0].caseID).not.toBe(attempts[2].caseID)
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
      expect(result.runs[0].attempts[0]).toMatchObject({
        seconds: 0,
        answer: '',
      })
      expect(result.runs[0].attempts[0].investigatorCost).toBeUndefined()
      expect(result.runs[0].attempts[0].graderCost).toBeUndefined()
      expect(result.notes.join(' ')).toContain('UI labels costs CAD')
    }
    const result = supported(kit())
    expect(result.runs[0].attempts[0].investigatorCost).toBeUndefined()
  })

  it('preserves original unknown fields, numeric lexemes and whitespace by caller contract', () => {
    const payload =
      '{ "evalId":"native", "config":{}, "results":{"version":3,"results":[]}, "unknown":{"exact":1.00,"signed":"opaque","nested":[true,null]}}\n'
    const source = Object.freeze(snapshot(null, { payload }))
    const result = projectEvalSnapshot(source)
    expect(result.status).toBe('supported')
    expect(source.payload).toBe(payload)
    expect(result).not.toHaveProperty('payload')
    expect(JSON.parse(source.payload).unknown).toEqual({
      exact: 1,
      signed: 'opaque',
      nested: [true, null],
    })
  })

  it('links only schema-established telemetry and explicit kit tool records', () => {
    const traceID = 'a'.repeat(32)
    const spanID = 'b'.repeat(16)
    const graderTrace = 'c'.repeat(32)
    const row = kitRow({
      response: {
        output: `Used bash in trace ${'d'.repeat(32)}`,
        metadata: {
          telemetry: {
            traceID,
            spanID,
            tools: [
              { spanID: 'e'.repeat(16), name: 'bash', error: true },
              { spanID: 'f'.repeat(16), name: 'unknown status' },
            ],
          },
        },
      },
      gradingResult: {
        score: 0.5,
        componentResults: [
          {
            metadata: {
              role: 'grader',
              metadata: {
                model: 'test/grader',
                telemetry: { traceID: graderTrace, spanID },
              },
            },
          },
        ],
      },
    })
    const attempt = supported(kit([row])).runs[0].attempts[0]
    expect(attempt.telemetry).toEqual({ traceID, spanID })
    expect(attempt.attemptTelemetry).toBeUndefined()
    expect(attempt.tools).toEqual([
      {
        name: 'bash',
        state: 'Error',
        message: '',
        telemetry: { traceID, spanID: 'e'.repeat(16) },
      },
    ])
    expect(attempt.assessments[0]).toMatchObject({
      evaluator: 'test/grader',
      telemetry: { traceID: graderTrace, spanID },
    })
    const native = supported(
      promptfoo([
        promptfooRow({ traceId: traceID, evaluationId: 'trace-evaluation' }),
      ])
    )
    expect(native.runs[0].attempts[0].telemetry).toEqual({
      traceID,
      spanID: undefined,
    })
    expect(native.rows[0].evaluationID).toBe('trace-evaluation')
    expect(native.runs[0].attempts[0].tools).toEqual([])
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
    expect(prose.runs[0].attempts[0].telemetry).toBeUndefined()
    expect(prose.runs[0].attempts[0].tools).toEqual([])
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
    const attempts = result.runs[0].attempts
    expect(attempts.slice(0, 4).map(attempt => attempt.answer)).toEqual([
      undefined,
      'null',
      'false',
      '0',
    ])
    expect(attempts[4].answer).toContain('file:///opaque')
    expect(attempts[5].answer!.length).toBeLessThan(33000)
    expect(result.notes.join(' ')).toContain('Long display text is truncated')
  })

  it('rejects malformed envelopes and selected row discriminators explicitly', () => {
    for (const value of [
      { manifest: {}, pilot: {} },
      { evalId: 'x', results: { version: 3, results: [] } },
      { ...promptfoo(), results: { version: 3, results: {} } },
      promptfoo([null as never]),
      promptfoo([promptfooRow({ repeatIndex: -1 })]),
      promptfoo([promptfooRow({ failureReason: '2' })]),
    ]) {
      expect(projectEvalSnapshot(snapshot(value)).status).toBe('error')
    }
    expect(projectEvalSnapshot(snapshot(null, { payload: '{' })).status).toBe(
      'error'
    )
    for (const value of [
      [],
      { custom: 'native' },
      { ...promptfoo(), results: { version: 2, results: [] } },
      {
        manifest: { models: [] },
        pilot: { results: { version: 4, results: [] } },
      },
    ]) {
      expect(projectEvalSnapshot(snapshot(value)).status).toBe('unsupported')
    }
    expect(
      projectEvalSnapshot(snapshot(promptfoo(), { format: 'jsonl' })).status
    ).toBe('unsupported')
    expect(supported(promptfoo(), { format: 'application/json' }).adapter).toBe(
      'promptfoo-v3'
    )
  })

  it('rejects unsafe/nonfinite numbers even in unknown fields and retains original text', () => {
    for (const lexeme of ['9007199254740993', '1e400', '-9007199254740993']) {
      const source = snapshot(null, {
        payload: `{"evalId":"x","config":{},"results":{"version":3,"results":[]},"unknown":${lexeme}}`,
      })
      const original = source.payload
      const result = projectEvalSnapshot(source)
      expect(result.status).toBe('error')
      if (result.status !== 'supported')
        expect(result.reason).toContain('unsafe integer')
      expect(source.payload).toBe(original)
    }
  })

  it('bounds payload, rows, nesting, node count and total display text', () => {
    expect(
      projectEvalSnapshot(
        snapshot(null, {
          payload: ' '.repeat(snapshotProjectionLimits.payloadCharacters + 1),
        })
      ).status
    ).toBe('error')
    expect(
      projectEvalSnapshot(
        snapshot(
          promptfoo(
            Array.from(
              { length: snapshotProjectionLimits.rows + 1 },
              () => ({})
            )
          )
        )
      ).status
    ).toBe('error')
    expect(
      projectEvalSnapshot(
        snapshot(null, { payload: '['.repeat(100) + '0' + ']'.repeat(100) })
      ).status
    ).toBe('error')
    expect(
      projectEvalSnapshot(
        snapshot({ unknown: Array(snapshotProjectionLimits.nodes + 1).fill(0) })
      ).status
    ).toBe('error')
    expect(
      projectEvalSnapshot(
        snapshot({
          ...promptfoo(),
          evalId: 'x'.repeat(snapshotProjectionLimits.identityCharacters + 1),
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
      result.runs[0].attempts.reduce(
        (sum, attempt) => sum + (attempt.answer?.length ?? 0),
        0
      )
    ).toBeLessThan(snapshotProjectionLimits.projectedCharacters + 1000)
  })
})
