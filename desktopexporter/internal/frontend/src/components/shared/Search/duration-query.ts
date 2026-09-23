import { parseDuration } from '@/utils/time'

const INT64_MAX = 9_223_372_036_854_775_807n

type DurationNormalization =
  { ok: true; value: string } | { ok: false; error: string }

type DurationListNormalization =
  { ok: true; value: string[] } | { ok: false; error: string; index?: number }

export function normalizeDuration(value: string): DurationNormalization {
  const nanoseconds = parseDuration(value)
  return nanoseconds === null || nanoseconds > INT64_MAX
    ? {
        ok: false,
        error: `Invalid duration: "${value}". Try "1s", "500ms", "2m", etc.`,
      }
    : { ok: true, value: nanoseconds.toString() }
}

export function normalizeDurationList(
  values: readonly string[]
): DurationListNormalization {
  if (values.length === 0) {
    return { ok: false, error: 'IN and NOT IN require a nonempty list' }
  }

  const normalized: string[] = []
  for (const [index, item] of values.entries()) {
    const result = normalizeDuration(item)
    if (!result.ok) {
      return {
        ok: false,
        error: `Invalid duration at list element ${index + 1}`,
        index,
      }
    }
    normalized.push(result.value)
  }
  return { ok: true, value: normalized }
}
