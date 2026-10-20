import {
  assessmentSummary,
  estimatedCost,
  type AssessmentDefinition,
  type EvalAttempt,
} from './model'

export function caseSummary(
  attempts: EvalAttempt[],
  definitions: AssessmentDefinition[] = []
) {
  let seconds = 0
  let timed = 0
  let calls = 0
  let failures = 0
  for (const attempt of attempts) {
    if (attempt.seconds !== undefined) {
      seconds += attempt.seconds
      timed++
    }
    calls += attempt.tools.length
    failures += attempt.tools.filter(tool => tool.state === 'Error').length
  }
  return {
    scores: Object.fromEntries(
      definitions.map(definition => [
        definition.id,
        assessmentSummary({ attempts }, definition),
      ])
    ),
    duration: {
      total: timed ? seconds : null,
      incomplete: timed < attempts.length,
    },
    calls,
    failures,
    investigatorCost: estimatedCost(attempts, 'investigatorCost'),
    graderCost: estimatedCost(attempts, 'graderCost'),
  }
}
