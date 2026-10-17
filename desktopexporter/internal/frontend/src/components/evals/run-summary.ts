import type { OutcomePart } from '@/components/shared/OutcomeBar.svelte'
import { assessmentSummary, estimatedCost, type EvalRun } from './model'

export function runSummary(run: EvalRun) {
  const definition = run.assessments.find(
    item => item.id === run.headlineAssessment
  )
  const groups = new Map<string, OutcomePart>()
  let toolCalls = 0
  let failedTools = 0
  let started = 0

  for (const attempt of run.attempts) {
    if (attempt.state !== 'Planned') started++
    toolCalls += attempt.tools.length
    failedTools += attempt.tools.filter(tool => tool.state === 'Error').length
    const assessment = attempt.assessments.find(
      item => item.id === definition?.id
    )
    let label = 'Unscored'
    let tone: OutcomePart['tone'] = 'neutral'
    if (definition && assessment?.value != null) {
      label = assessment.label
      tone =
        assessment.value === definition.maximum
          ? 'success'
          : assessment.value === 0
            ? 'error'
            : 'warning'
    } else if (attempt.state === 'Planned') {
      label = 'Planned'
    } else if (attempt.state === 'Error') {
      label = 'Execution error'
    }
    const key = `${tone}:${label}`
    const group = groups.get(key)
    if (group) group.count++
    else groups.set(key, { label, tone, count: 1 })
  }

  const order = { success: 0, warning: 1, error: 2, neutral: 3 }
  return {
    definition,
    assessment: definition ? assessmentSummary(run, definition) : null,
    parts: [...groups.values()].sort((a, b) => order[a.tone] - order[b.tone]),
    planned: run.attempts.length,
    assessed: run.attempts.filter(attempt => attempt.state === 'Complete')
      .length,
    started,
    toolCalls,
    failedTools,
    investigatorCost: estimatedCost(run.attempts, 'investigatorCost'),
  }
}
