import type { EvalResult } from './model'

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

function sortValue(result: EvalResult, key: ResultSortKey) {
  switch (key) {
    case 'name':
      return result.name
    case 'state':
      return result.state
    case 'repetition':
      return result.repetition
    case 'started': {
      const time = result.startedAt ? Date.parse(result.startedAt) : NaN
      return Number.isFinite(time) ? time : undefined
    }
    case 'seconds':
      return result.seconds
    case 'tools':
      return result.tools.length
    case 'failures':
      return result.tools.filter(tool => tool.state === 'Error').length
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
  if (!sort)
    return [...results].sort((a, b) => {
      const start = (result: EvalResult) =>
        result.startedAt ? Date.parse(result.startedAt) : NaN
      const left = start(a),
        right = start(b)
      if (!Number.isFinite(left)) return Number.isFinite(right) ? 1 : 0
      if (!Number.isFinite(right)) return -1
      return left - right
    })
  const direction = sort.direction === 'asc' ? 1 : -1
  return [...results].sort((left, right) => {
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
