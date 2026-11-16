import {
  assessmentSummary,
  estimatedCost,
  evalToolCounts,
  type AssessmentDefinition,
  type EvalResult,
} from './model'

export function caseSummary(
  results: EvalResult[],
  definitions: AssessmentDefinition[] = []
) {
  let calls = 0
  let failures = 0
  for (const result of results) {
    calls += evalToolCounts(result).total
    failures += evalToolCounts(result).failed
  }
  return {
    scores: Object.fromEntries(
      definitions.map(definition => [
        definition.id,
        assessmentSummary({ results }, definition),
      ])
    ),
    calls,
    failures,
    investigatorCost: estimatedCost(results, 'investigatorCost'),
    graderCost: estimatedCost(results, 'graderCost'),
  }
}
