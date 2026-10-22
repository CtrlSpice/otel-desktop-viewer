import type { OutcomePart } from '@/components/shared/OutcomeBar.svelte'
import { assessmentSummary, estimatedCost, type EvalRun } from './model'

/** Card data is independent of the full rep/configuration payload. */
export type EvalRunCardData = Pick<
  EvalRun,
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
  repCount: number
}

export function toRunCard(run: EvalRun): EvalRunCardData {
  const summary = runSummary(run)
  return {
    id: run.id,
    name: run.name,
    evaluation: run.evaluation,
    source: run.source,
    state: run.state,
    stateReason: run.stateReason,
    pauseReason: run.pauseReason,
    startedAt: run.startedAt,
    outcomes: summary.parts,
    outcomeLabel: summary.definition?.name ?? 'Results',
    repCount: summary.planned,
  }
}

export function runSummary(run: EvalRun) {
  const definition = run.assessments.find(
    item => item.id === run.headlineAssessment
  )
  const groups = new Map<string, OutcomePart>()
  let toolCalls = 0
  let failedTools = 0
  let started = 0

  for (const rep of run.reps) {
    if (rep.state !== 'Planned') started++
    toolCalls += rep.tools.length
    failedTools += rep.tools.filter(tool => tool.state === 'Error').length
    const assessment = rep.assessments.find(item => item.id === definition?.id)
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
    } else if (rep.state === 'Planned') {
      label = 'Planned'
    } else if (rep.state === 'Error') {
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
    assessment: definition ? assessmentSummary(run, definition) : null,
    parts: [...groups.values()].sort((a, b) => order[a.tone] - order[b.tone]),
    planned: run.reps.length,
    assessed: run.reps.filter(rep => rep.state === 'Complete').length,
    started,
    toolCalls,
    failedTools,
    investigatorCost: estimatedCost(run.reps, 'investigatorCost'),
  }
}
