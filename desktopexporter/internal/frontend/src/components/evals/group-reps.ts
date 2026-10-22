import type { EvalRep } from './model'
import { sortReps, type RepSort } from './sort-reps'

export function completionState(
  reps: readonly Pick<EvalRep, 'state'>[]
): EvalRep['state'] {
  if (reps.some(rep => rep.state === 'Error')) return 'Error'
  if (reps.every(rep => rep.state === 'Planned')) return 'Planned'
  if (reps.every(rep => rep.state === 'Complete')) return 'Complete'
  return 'Running'
}

export function completionLabel(
  state: EvalRep['state'] | 'Unknown',
  internalFault = false
) {
  return state === 'Error'
    ? internalFault
      ? 'Internal fault'
      : 'Span error'
    : state
}

export function completionTone(
  state: EvalRep['state'] | 'Unknown',
  internalFault = false
) {
  switch (state) {
    case 'Complete':
      return 'complete'
    case 'Running':
      return 'running'
    case 'Error':
      return internalFault ? 'papaya' : 'error'
    case 'Planned':
    case 'Unknown':
      return 'neutral'
  }
}

/** Rank cases by their first sorted visible rep, then sort within each case.
 * Full case membership determines completion and grouping, even while filtered. */
export function groupReps(
  reps: readonly EvalRep[],
  allReps: readonly EvalRep[],
  sort?: RepSort
) {
  const cases = new Map<string, EvalRep[]>()
  for (const rep of allReps) {
    const members = cases.get(rep.caseID) ?? []
    members.push(rep)
    cases.set(rep.caseID, members)
  }
  const visible = new Map<string, EvalRep[]>()
  for (const rep of sortReps(reps, sort)) {
    const members = visible.get(rep.caseID) ?? []
    members.push(rep)
    visible.set(rep.caseID, members)
  }
  return [...visible].map(([id, members]) => {
    const all = cases.get(id) ?? members
    return {
      id,
      name: all[0].name,
      state: completionState(all),
      allReps: all,
      reps: members,
    }
  })
}
