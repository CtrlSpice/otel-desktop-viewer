import {
  assessmentSummary,
  estimatedCost,
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
    calls += result.tools.length
    failures += result.tools.filter(tool => tool.state === 'Error').length
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
