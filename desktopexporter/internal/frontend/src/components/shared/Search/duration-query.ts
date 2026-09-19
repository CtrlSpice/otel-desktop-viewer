import { parseDuration } from '@/utils/time'
import type { QueryNode } from './queryTree'

const INT64_MAX = 9_223_372_036_854_775_807n

function normalizeDuration(value: string): string | null {
  const nanoseconds = parseDuration(value)
  if (nanoseconds === null || nanoseconds > INT64_MAX) return null
  return nanoseconds.toString()
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
  if (!('name' in node.query.field) || node.query.field.name !== 'duration') {
    return null
  }

  if (
    node.query.operator.symbol === 'IN' ||
    node.query.operator.symbol === 'NOT IN'
  ) {
    let values: unknown
    try {
      values = JSON.parse(node.query.value)
    } catch {
      return 'Invalid duration list'
    }
    if (!Array.isArray(values)) return 'Invalid duration list'
    if (values.length === 0) return 'IN and NOT IN require a nonempty list'

    const normalized: string[] = []
    for (const [index, value] of values.entries()) {
      if (typeof value !== 'string') {
        return `Invalid duration at list element ${index + 1}`
      }
      const nanoseconds = normalizeDuration(value)
      if (nanoseconds === null) {
        return `Invalid duration at list element ${index + 1}`
      }
      normalized.push(nanoseconds)
    }
    node.query.value = JSON.stringify(normalized)
    return null
  }

  const nanoseconds = normalizeDuration(node.query.value)
  if (nanoseconds === null) {
    return `Invalid duration: "${node.query.value}". Try "1s", "500ms", "2m", etc.`
  }
  node.query.value = nanoseconds
  return null
}
