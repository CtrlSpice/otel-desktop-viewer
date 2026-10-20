import type { EvalAttempt } from './model'
import { sortAttempts, type AttemptSort } from './sort-attempts'

export function completionState(
  attempts: readonly EvalAttempt[]
): EvalAttempt['state'] {
  if (attempts.some(attempt => attempt.state === 'Error')) return 'Error'
  if (attempts.every(attempt => attempt.state === 'Planned')) return 'Planned'
  if (attempts.every(attempt => attempt.state === 'Complete')) return 'Complete'
  return 'Running'
}

export function completionTone(state: EvalAttempt['state']) {
  switch (state) {
    case 'Complete':
      return 'complete'
    case 'Running':
      return 'running'
    case 'Error':
      return 'papaya'
    case 'Planned':
      return 'neutral'
  }
}

/** Rank cases by their first sorted visible attempt, then sort within each case.
 * Full case membership determines completion and grouping, even while filtered. */
export function groupAttempts(
  attempts: readonly EvalAttempt[],
  allAttempts: readonly EvalAttempt[],
  sort?: AttemptSort
) {
  const cases = new Map<string, EvalAttempt[]>()
  for (const attempt of allAttempts) {
    const members = cases.get(attempt.caseID) ?? []
    members.push(attempt)
    cases.set(attempt.caseID, members)
  }
  const visible = new Map<string, EvalAttempt[]>()
  for (const attempt of sortAttempts(attempts, sort)) {
    const members = visible.get(attempt.caseID) ?? []
    members.push(attempt)
    visible.set(attempt.caseID, members)
  }
  return [...visible].map(([id, members]) => {
    const all = cases.get(id) ?? members
    return {
      id,
      name: all[0].name,
      state: completionState(all),
      allAttempts: all,
      attempts: members,
    }
  })
}
