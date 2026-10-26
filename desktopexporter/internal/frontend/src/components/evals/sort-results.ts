import { evalToolCounts, type EvalResult } from './model'
import { startedTimestamp } from './experiment-sort'

export type ResultSortKey =
  | 'name'
  | 'state'
  | 'repetition'
  | 'started'
  | 'seconds'
  | 'tools'
  | 'failures'
  | 'investigatorCost'
  | 'graderCost'
  | 'cost'
  | `assessment:${string}`

export type ResultSort = {
  key: ResultSortKey
  direction: 'asc' | 'desc'
}

function numericSortValue(
  result: EvalResult,
  key: Exclude<ResultSortKey, 'name' | 'state' | 'started'>
) {
  switch (key) {
    case 'repetition':
      return result.repetition
    case 'seconds':
      return result.seconds
    case 'tools':
      return evalToolCounts(result).total
    case 'failures':
      return evalToolCounts(result).failed
    case 'investigatorCost':
      return result.investigatorCost
    case 'graderCost':
      return result.graderCost
    case 'cost':
      return result.investigatorCost === undefined &&
        result.graderCost === undefined
        ? undefined
        : (result.investigatorCost ?? 0) + (result.graderCost ?? 0)
    default:
      return result.assessments.find(
        score => score.id === key.slice('assessment:'.length)
      )?.value
  }
}

/** Stable local ordering; unknown measurements stay last in either direction. */
export function sortResults(
  results: readonly EvalResult[],
  sort?: ResultSort
): EvalResult[] {
  const direction = sort?.direction === 'desc' ? -1 : 1
  const key = sort?.key ?? 'started'
  if (key === 'started')
    return [...results].sort((left, right) => {
      const a = left.startedTime ?? startedTimestamp(left.startedAt)
      const b = right.startedTime ?? startedTimestamp(right.startedAt)
      if (a === null) return b === null ? 0 : 1
      if (b === null) return -1
      return (a < b ? -1 : a > b ? 1 : 0) * direction
    })
  if (key === 'name' || key === 'state')
    return [...results].sort((left, right) => {
      const a = left[key]
      const b = right[key]
      if (a == null) return b == null ? 0 : 1
      if (b == null) return -1
      return a.localeCompare(b, undefined, { numeric: true }) * direction
    })
  return [...results].sort((left, right) => {
    const a = numericSortValue(left, key)
    const b = numericSortValue(right, key)
    if (a == null) return b == null ? 0 : 1
    if (b == null) return -1
    return (a - b) * direction
  })
}
