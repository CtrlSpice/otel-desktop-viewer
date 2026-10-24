import type { EvalResult } from './model'
import { sortResults, type ResultSort } from './sort-results'

export function completionState(
  results: readonly Pick<EvalResult, 'state'>[]
): EvalResult['state'] {
  if (results.some(result => result.state === 'Error')) return 'Error'
  if (results.every(result => result.state === 'Planned')) return 'Planned'
  if (results.every(result => result.state === 'Complete')) return 'Complete'
  return 'Running'
}

export function completionLabel(
  state: EvalResult['state'] | 'Unknown',
  internalFault = false
) {
  return state === 'Error'
    ? internalFault
      ? 'Internal fault'
      : 'Span error'
    : state
}

export function completionTone(
  state: EvalResult['state'] | 'Unknown',
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

/** Rank cases by their first sorted visible result, then sort within each case.
 * Full case membership determines completion and grouping, even while filtered. */
export function groupResults(
  results: readonly EvalResult[],
  allResults: readonly EvalResult[],
  sort?: ResultSort
) {
  const cases = new Map<string, EvalResult[]>()
  for (const result of allResults) {
    const members = cases.get(result.caseID) ?? []
    members.push(result)
    cases.set(result.caseID, members)
  }
  const visible = new Map<string, EvalResult[]>()
  for (const result of sortResults(results, sort)) {
    const members = visible.get(result.caseID) ?? []
    members.push(result)
    visible.set(result.caseID, members)
  }
  return [...visible].map(([id, members]) => {
    const all = cases.get(id) ?? members
    return {
      id,
      name: all[0].name,
      state: completionState(all),
      allResults: all,
      results: members,
    }
  })
}
