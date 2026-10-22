import { describe, expect, it } from 'vitest'
import {
  projectPromptfooDataset,
  evalProjectionLimits,
} from './promptfoo-adapter'
import {
  evalSourceFieldCopy,
  evalSourceFieldText,
  evalSourceRunCopy,
} from './eval-source-copy'

function project(rows: Record<string, unknown>[]) {
  const results = { version: 3, results: rows }
  const payload = JSON.stringify({ evalId: 'test', config: {}, results })
  const projection = projectPromptfooDataset({
    id: 'dataset',
    name: 'test',
    source: 'test',
    format: 'json',
    payload,
  })
  if (projection.status !== 'supported') throw new Error(projection.reason)
  const copy = evalSourceFieldCopy(payload, projection.rows)
  return { payload, projection, copy }
}

describe('exact dataset field copy', () => {
  it('keeps per-field previews bounded while copying all native prose, including Unicode', async () => {
    const value = '🙂\n"\\é'.repeat(6000)
    const { projection, copy } = project([
      {
        description: value,
        prompt: { raw: value },
        response: { output: value },
        gradingResult: { reason: value },
        failureReason: 2,
        error: value,
      },
    ])
    const rep = projection.runs[0].reps[0]
    const previews = [
      rep.name,
      rep.question,
      rep.answer,
      rep.assessments[0].explanation,
      rep.error,
    ]
    for (const preview of previews)
      expect(preview).toBe(value.slice(0, 32768) + '… [see original JSON]')
    for (const field of [
      'name',
      'question',
      'answer',
      'explanation',
      'error',
    ] as const) {
      const exact = await copy(rep.id, field)!(new AbortController().signal)
      expect(exact).toBe(value)
      expect(exact).not.toContain('… [see original JSON]')
    }
  })

  it('copies a late row exactly after the aggregate projection budget is exhausted', async () => {
    const value = 'x'.repeat(evalProjectionLimits.textCharacters)
    const { projection, copy } = project(
      Array.from({ length: 40 }, (_, index) => ({
        id: `row-${index}`,
        response: { output: value },
        prompt: { raw: 'Late question' },
        gradingResult: { reason: 'Late explanation' },
      }))
    )
    const rep = projection.runs.at(-1)!.reps[0]
    expect(rep.answer).toBe('… [see original JSON]')
    expect(rep.question).toBe('… [see original JSON]')
    expect(rep.assessments[0].explanation).toBe('… [see original JSON]')
    expect(await copy(rep.id, 'answer')!(new AbortController().signal)).toBe(
      value
    )
    expect(await copy(rep.id, 'question')!(new AbortController().signal)).toBe(
      'Late question'
    )
    expect(
      await copy(rep.id, 'explanation')!(new AbortController().signal)
    ).toBe('Late explanation')
  })

  it('distinguishes missing output, null, false, zero and an explicitly empty string', async () => {
    const { projection, copy } = project(
      [undefined, null, false, 0, ''].map(output => ({ response: { output } }))
    )
    const reps = projection.runs.flatMap(run => run.reps)
    expect(reps.map(rep => rep.answer)).toEqual([
      undefined,
      'null',
      'false',
      '0',
      '',
    ])
    expect(copy(reps[0].id, 'answer')).toBeUndefined()
    expect(copy(reps[0].id, 'question')).toBeUndefined()
    for (const [index, expected] of ['null', 'false', '0', ''].entries())
      expect(
        await copy(reps[index + 1].id, 'answer')!(new AbortController().signal)
      ).toBe(expected)
  })

  it('records the exact selected fallback path, preserving empty native strings', async () => {
    const { projection, copy } = project([
      {
        prompt: { raw: '' },
        failureReason: 1,
        error: 'fallback',
        gradingResult: { reason: '' },
      },
      {
        prompt: { raw: 'fallback' },
        failureReason: 1,
        error: 'failure',
      },
      { failureReason: 2, response: { error: '' } },
    ])
    const reps = projection.runs.flatMap(run => run.reps)
    const signal = new AbortController().signal
    expect(await copy(reps[0].id, 'question')!(signal)).toBe('')
    expect(await copy(reps[0].id, 'explanation')!(signal)).toBe('')
    expect(await copy(reps[1].id, 'question')!(signal)).toBe('fallback')
    expect(await copy(reps[1].id, 'explanation')!(signal)).toBe('failure')
    expect(await copy(reps[2].id, 'error')!(signal)).toBe('')
  })

  it('extracts structured subtrees without losing unknown fields or numeric lexemes', () => {
    const subtree =
      '{ "unknown": [1.00, -0, 0.1234567890123456789, 9007199254740993], "url":"file:///never-open" }'
    const payload = `{"results":{"results":[{"response":{"output":${subtree}}}]}}`
    expect(
      evalSourceFieldText(payload, '/results/results/0/response/output')
    ).toBe(subtree)
    expect(
      evalSourceFieldText(
        '{"a/b":{"~":["old","\\uD83D\\uDE42"]}}',
        '/a~1b/~0/1'
      )
    ).toBe('🙂')
    expect(evalSourceFieldText('{"x":1,"x":-0.00}', '/x')).toBe('-0.00')
    expect(() => evalSourceFieldText('{"x":null}', '/missing')).toThrow(
      'Source field is missing'
    )
  })

  it('copies a supported structured answer without rounding its fractional numbers', async () => {
    const value =
      '{ "extra": [1.00, -0.00, 0.1234567890123456789, 1e-3], "ref": "file:///unresolved" }'
    const payload = `{"evalId":"x","config":{},"results":{"version":3,"results":[{"response":{"output":${value}}}]}}`
    const projection = projectPromptfooDataset({
      id: 's',
      name: 'test',
      source: 'test',
      format: 'json',
      payload,
    })
    if (projection.status !== 'supported') throw new Error(projection.reason)
    const rep = projection.runs[0].reps[0]
    expect(rep.answer).not.toBe(value)
    expect(
      await evalSourceFieldCopy(payload, projection.rows)(rep.id, 'answer')!(
        new AbortController().signal
      )
    ).toBe(value)
  })

  it('copies full native run text and reconstructs the derived run label from full operands', async () => {
    const description = 'evaluation'.repeat(5000)
    const label = 'prompt'.repeat(7000)
    const source = {
      id: 's',
      name: 'fallback',
      source: 'source'.repeat(7000),
      format: 'json',
      payload: JSON.stringify({
        evalId: 'x',
        config: { description },
        results: {
          version: 3,
          results: [
            { provider: { id: 'id', label: 'provider' }, prompt: { label } },
          ],
        },
      }),
    }
    const projection = projectPromptfooDataset(source)
    if (projection.status !== 'supported') throw new Error(projection.reason)
    const run = projection.runs[0]
    const copy = evalSourceRunCopy(source, projection.runSources)
    const signal = new AbortController().signal
    expect(run.name).toBe('provider')
    expect(await copy(run.id, 'name')!(signal)).toBe('provider')
    expect(await copy(run.id, 'evaluation')!(signal)).toBe(description)
    expect(await copy(run.id, 'prompt')!(signal)).toBe(label)
    expect(await copy(run.id, 'source')!(signal)).toBe(source.source)
  })

  it('never invents tool output from an imported telemetry identity', () => {
    const { projection } = project([
      {
        response: {
          metadata: {
            telemetry: {
              traceID: 'a'.repeat(32),
              spanID: 'b'.repeat(16),
              tools: [{ name: 'tool', spanID: 'c'.repeat(16), error: false }],
            },
          },
        },
      },
    ])
    expect(projection.runs[0].reps[0].tools).toEqual([])
  })
})
