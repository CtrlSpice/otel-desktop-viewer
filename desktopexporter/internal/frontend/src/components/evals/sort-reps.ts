import type { EvalRep } from './model'

export type RepSortKey =
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

export type RepSort = {
  key: RepSortKey
  direction: 'asc' | 'desc'
}

function sortValue(rep: EvalRep, key: RepSortKey) {
  switch (key) {
    case 'name':
      return rep.name
    case 'state':
      return rep.state
    case 'repetition':
      return rep.repetition
    case 'started': {
      const time = rep.startedAt ? Date.parse(rep.startedAt) : NaN
      return Number.isFinite(time) ? time : undefined
    }
    case 'seconds':
      return rep.seconds
    case 'tools':
      return rep.tools.length
    case 'failures':
      return rep.tools.filter(tool => tool.state === 'Error').length
    case 'investigatorCost':
      return rep.investigatorCost
    case 'graderCost':
      return rep.graderCost
    case 'cost':
      return rep.investigatorCost === undefined && rep.graderCost === undefined
        ? undefined
        : (rep.investigatorCost ?? 0) + (rep.graderCost ?? 0)
    default:
      return rep.assessments.find(
        score => score.id === key.slice('assessment:'.length)
      )?.value
  }
}

/** Stable local ordering; unknown measurements stay last in either direction. */
export function sortReps(reps: readonly EvalRep[], sort?: RepSort): EvalRep[] {
  if (!sort)
    return [...reps].sort((a, b) => {
      const start = (rep: EvalRep) =>
        rep.startedAt ? Date.parse(rep.startedAt) : NaN
      const left = start(a),
        right = start(b)
      if (!Number.isFinite(left)) return Number.isFinite(right) ? 1 : 0
      if (!Number.isFinite(right)) return -1
      return left - right
    })
  const direction = sort.direction === 'asc' ? 1 : -1
  return [...reps].sort((left, right) => {
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
