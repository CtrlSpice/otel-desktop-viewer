import type { OutcomePart } from '@/components/shared/OutcomeBar.svelte'
import type { EvalExperimentCardData } from './experiment-summary'

export type ExperimentOutcomeOption = OutcomePart & { key: string }

export function outcomeKey(part: Pick<OutcomePart, 'tone' | 'label'>) {
  return `${part.tone}:${part.label}`
}

/** Count each experiment once per category, regardless of its number of results. */
export function experimentOutcomeOptions(
  experiments: readonly Pick<EvalExperimentCardData, 'outcomes'>[]
): ExperimentOutcomeOption[] {
  const options = new Map<string, ExperimentOutcomeOption>()
  for (const experiment of experiments) {
    for (const part of experiment.outcomes) {
      const key = outcomeKey(part)
      const option = options.get(key)
      if (option) option.count++
      else options.set(key, { ...part, key, count: 1 })
    }
  }
  const order = { success: 0, warning: 1, error: 2, papaya: 3, neutral: 4 }
  return [...options.values()].sort((a, b) => order[a.tone] - order[b.tone])
}

/** URL selections must name categories in the current source. */
export function readExperimentOutcomeFilter(
  value: string | undefined,
  options: ExperimentOutcomeOption[]
): string[] {
  if (!value) return []
  try {
    const parsed: unknown = JSON.parse(value)
    return Array.isArray(parsed)
      ? options.flatMap(option =>
          parsed.includes(option.key) ? [option.key] : []
        )
      : []
  } catch {
    return []
  }
}
