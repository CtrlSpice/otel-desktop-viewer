import {
  assessmentSummary,
  estimatedCost,
  type AssessmentDefinition,
  type EvalRep,
} from './model'

export function caseSummary(
  reps: EvalRep[],
  definitions: AssessmentDefinition[] = []
) {
  let seconds = 0
  let timed = 0
  let calls = 0
  let failures = 0
  for (const rep of reps) {
    if (rep.seconds !== undefined) {
      seconds += rep.seconds
      timed++
    }
    calls += rep.tools.length
    failures += rep.tools.filter(tool => tool.state === 'Error').length
  }
  return {
    scores: Object.fromEntries(
      definitions.map(definition => [
        definition.id,
        assessmentSummary({ reps }, definition),
      ])
    ),
    duration: {
      total: timed ? seconds : null,
      incomplete: timed < reps.length,
    },
    calls,
    failures,
    investigatorCost: estimatedCost(reps, 'investigatorCost'),
    graderCost: estimatedCost(reps, 'graderCost'),
  }
}
