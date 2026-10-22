import type { OutcomePart } from '@/components/shared/OutcomeBar.svelte'
import { assessmentSummary, estimatedCost, type EvalExperiment } from './model'

/** Card data is independent of the full result/configuration payload. */
export type EvalExperimentCardData = Pick<
  EvalExperiment,
  | 'id'
  | 'name'
  | 'evaluation'
  | 'source'
  | 'state'
  | 'stateReason'
  | 'pauseReason'
  | 'startedAt'
> & {
  outcomes: OutcomePart[]
  outcomeLabel: string
  resultCount: number
}

export function toExperimentCard(
  experiment: EvalExperiment
): EvalExperimentCardData {
  const summary = experimentSummary(experiment)
  return {
    id: experiment.id,
    name: experiment.name,
    evaluation: experiment.evaluation,
    source: experiment.source,
    state: experiment.state,
    stateReason: experiment.stateReason,
    pauseReason: experiment.pauseReason,
    startedAt: experiment.startedAt,
    outcomes: summary.parts,
    outcomeLabel: summary.definition?.name ?? 'Results',
    resultCount: summary.planned,
  }
}

export function experimentSummary(experiment: EvalExperiment) {
  const definition = experiment.assessments.find(
    item => item.id === experiment.headlineAssessment
  )
  const groups = new Map<string, OutcomePart>()
  let toolCalls = 0
  let failedTools = 0
  let started = 0

  for (const result of experiment.results) {
    if (result.state !== 'Planned') started++
    toolCalls += result.tools.length
    failedTools += result.tools.filter(tool => tool.state === 'Error').length
    const assessment = result.assessments.find(
      item => item.id === definition?.id
    )
    let label = 'Unscored'
    let tone: OutcomePart['tone'] = 'neutral'
    if (assessment?.pass !== undefined) {
      label = assessment.label
      tone = assessment.pass ? 'success' : 'error'
    } else if (definition?.maximum !== undefined && assessment?.value != null) {
      label = assessment.label
      tone =
        assessment.value === definition.maximum
          ? 'success'
          : assessment.value === 0
            ? 'error'
            : 'warning'
    } else if (result.state === 'Planned') {
      label = 'Planned'
    } else if (result.state === 'Error') {
      label = 'Internal fault'
      tone = 'papaya'
    }
    const key = `${tone}:${label}`
    const group = groups.get(key)
    if (group) group.count++
    else groups.set(key, { label, tone, count: 1 })
  }

  const order = { success: 0, warning: 1, error: 2, papaya: 3, neutral: 4 }
  return {
    definition,
    assessment: definition ? assessmentSummary(experiment, definition) : null,
    parts: [...groups.values()].sort((a, b) => order[a.tone] - order[b.tone]),
    planned: experiment.results.length,
    assessed: experiment.results.filter(result => result.state === 'Complete')
      .length,
    started,
    toolCalls,
    failedTools,
    investigatorCost: estimatedCost(experiment.results, 'investigatorCost'),
  }
}
