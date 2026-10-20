import type { EvalAttempt } from './model'

export type AttemptSortKey =
  | 'name'
  | 'state'
  | 'repetition'
  | 'seconds'
  | 'tools'
  | 'failures'
  | 'investigatorCost'
  | 'graderCost'
  | 'cost'
  | `assessment:${string}`

export type AttemptSort = {
  key: AttemptSortKey
  direction: 'asc' | 'desc'
}

function sortValue(attempt: EvalAttempt, key: AttemptSortKey) {
  switch (key) {
    case 'name':
      return attempt.name
    case 'state':
      return attempt.state
    case 'repetition':
      return attempt.repetition
    case 'seconds':
      return attempt.seconds
    case 'tools':
      return attempt.tools.length
    case 'failures':
      return attempt.tools.filter(tool => tool.state === 'Error').length
    case 'investigatorCost':
      return attempt.investigatorCost
    case 'graderCost':
      return attempt.graderCost
    case 'cost':
      return attempt.investigatorCost === undefined &&
        attempt.graderCost === undefined
        ? undefined
        : (attempt.investigatorCost ?? 0) + (attempt.graderCost ?? 0)
    default:
      return attempt.assessments.find(
        score => score.id === key.slice('assessment:'.length)
      )?.value
  }
}

/** Stable local ordering; unknown measurements stay last in either direction. */
export function sortAttempts(
  attempts: readonly EvalAttempt[],
  sort?: AttemptSort
): EvalAttempt[] {
  if (!sort) return [...attempts]
  const direction = sort.direction === 'asc' ? 1 : -1
  return [...attempts].sort((left, right) => {
    const a = sortValue(left, sort.key)
    const b = sortValue(right, sort.key)
    if (a == null) return b == null ? 0 : 1
    if (b == null) return -1
    const comparison =
      typeof a === 'number' && typeof b === 'number'
        ? a - b
        : String(a).localeCompare(String(b), undefined, { numeric: true })
    return comparison * direction
  })
}
