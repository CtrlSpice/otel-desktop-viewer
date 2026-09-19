import { parseDuration } from '@/utils/time'
import type { Query, QueryNode } from './queryTree'

const INT64_MAX = 9_223_372_036_854_775_807n

type DurationNormalization =
  { ok: true; value: string } | { ok: false; error: string }

function normalizeDuration(value: string): DurationNormalization {
  const nanoseconds = parseDuration(value)
  return nanoseconds === null || nanoseconds > INT64_MAX
    ? {
        ok: false,
        error: `Invalid duration: "${value}". Try "1s", "500ms", "2m", etc.`,
      }
    : { ok: true, value: nanoseconds.toString() }
}

function normalizeDurationList(value: string): DurationNormalization {
  let values: unknown
  try {
    values = JSON.parse(value)
  } catch {
    return { ok: false, error: 'Invalid duration list' }
  }
  if (!Array.isArray(values)) {
    return { ok: false, error: 'Invalid duration list' }
  }
  if (values.length === 0) {
    return { ok: false, error: 'IN and NOT IN require a nonempty list' }
  }

  const normalized: string[] = []
  for (const [index, item] of values.entries()) {
    if (typeof item !== 'string') {
      return {
        ok: false,
        error: `Invalid duration at list element ${index + 1}`,
      }
    }
    const result = normalizeDuration(item)
    if (!result.ok) {
      return {
        ok: false,
        error: `Invalid duration at list element ${index + 1}`,
      }
    }
    normalized.push(result.value)
  }
  return { ok: true, value: JSON.stringify(normalized) }
}

function normalizeDurationQuery(query: Query): DurationNormalization {
  return query.operator.symbol === 'IN' || query.operator.symbol === 'NOT IN'
    ? normalizeDurationList(query.value)
    : normalizeDuration(query.value)
}

/** Normalizes mapper-resolved duration values before sending their string wire form. */
export function normalizeDurationValues(node: QueryNode): string | null {
  if (node.type === 'group') {
    for (const child of node.group.children) {
      const error = normalizeDurationValues(child)
      if (error) return error
    }
    return null
  }
  if (
    node.query.field.searchScope !== 'field' ||
    !('name' in node.query.field) ||
    node.query.field.name !== 'duration'
  ) {
    return null
  }

  const result = normalizeDurationQuery(node.query)
  if (!result.ok) return result.error
  node.query.value = result.value
  return null
}
