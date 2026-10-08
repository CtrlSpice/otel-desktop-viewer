import cases from './tasks.json' with { type: 'json' }
import { record } from './runtime.ts'

export function matches(actual: unknown, expected: unknown): boolean {
  if (typeof expected === 'number') {
    return (
      typeof actual === 'number' &&
      Number.isFinite(actual) &&
      (Number.isInteger(expected)
        ? actual === expected
        : Math.abs(actual - expected) < 1e-8)
    )
  }
  if (Array.isArray(expected)) {
    if (!Array.isArray(actual) || actual.length !== expected.length)
      return false
    const remaining = [...actual]
    return expected.every(want => {
      const i = remaining.findIndex(got => matches(got, want))
      if (i < 0) return false
      remaining.splice(i, 1)
      return true
    })
  }
  if (expected !== null && typeof expected === 'object') {
    if (actual === null || typeof actual !== 'object') return false
    const fields = Object.fromEntries(Object.entries(actual))
    return Object.entries(expected).every(([key, value]) =>
      matches(fields[key], value)
    )
  }
  return actual === expected
}

export function parseAnswer(output: string): unknown {
  const text = output.trim()
  try {
    return JSON.parse(text)
  } catch {}
  const fences = [...text.matchAll(/```(?:json)?\s*\n([\s\S]*?)```/g)]
  if (fences.length === 1) return JSON.parse(fences[0][1])
  throw new Error('No single JSON answer; retain output for manual grading.')
}

export default function grade(
  output: string,
  context: { vars: { task_id?: unknown } }
) {
  const task = cases.find(c => c.id === context.vars.task_id)
  if (!task) throw new Error('Unknown task')
  let answer
  try {
    answer = record(parseAnswer(output))
  } catch (error) {
    return {
      pass: false,
      score: 0,
      reason: error instanceof Error ? error.message : String(error),
    }
  }
  const fields = Object.keys(task.expected)
  const expected = record(task.expected)
  const failed = fields.filter(key => !matches(answer[key], expected[key]))
  if (
    !answer.evidence ||
    (Array.isArray(answer.evidence) && !answer.evidence.length)
  )
    failed.push('evidence')
  return {
    pass: failed.length === 0,
    score: Math.max(0, (fields.length - failed.length) / fields.length),
    reason: failed.length
      ? 'Incorrect or missing: ' + failed.join(', ')
      : 'All expected facts match; evidence provenance and command choices require transcript review.',
  }
}
