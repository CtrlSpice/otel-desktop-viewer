import { record } from './runtime.ts'
import { responseCostUsd, usageCount } from './pricing.ts'
import type { PricingSnapshot, UsageStep } from './pricing.ts'

const sum = (values: unknown[]) => {
  if (values.some(value => usageCount(value) === undefined)) return null
  const total = values.reduce<number>(
    (total, value) => total + (usageCount(value) ?? 0),
    0
  )
  return Number.isSafeInteger(total) ? total : null
}

export function median(values: number[]) {
  if (!values.length) return null
  const sorted = [...values].sort((a, b) => a - b)
  const middle = Math.floor(sorted.length / 2)
  return sorted.length % 2
    ? sorted[middle]
    : (sorted[middle - 1] + sorted[middle]) / 2
}

export function modelSummaries(rows: unknown[], pricing?: PricingSnapshot) {
  const groups = new Map<string, Record<string, unknown>[]>()
  for (const value of rows) {
    const row = record(value)
    const response = row.response === undefined ? {} : record(row.response)
    const metadata =
      response.metadata === undefined ? {} : record(response.metadata)
    const model = metadata.model ?? record(row.provider).label
    if (typeof model !== 'string') throw new Error('Missing result model')
    const group = groups.get(model) ?? []
    group.push(metadata)
    groups.set(model, group)
  }
  return [...groups].map(([model, attempts]) => {
    const usages = attempts.map(attempt =>
      attempt.usage === undefined ? {} : record(attempt.usage)
    )
    const times = attempts.map(attempt => attempt.elapsedSeconds)
    const validTimes = times.filter(
      (value): value is number =>
        typeof value === 'number' && Number.isFinite(value) && value >= 0
    )
    let costUsd = 0
    let pricedResponses = 0
    let complete = true
    for (const attempt of attempts) {
      if (!Array.isArray(attempt.usageSteps) || !attempt.usageSteps.length) {
        complete = false
        continue
      }
      for (const raw of attempt.usageSteps) {
        const value = record(raw)
        const step: UsageStep = {
          input: usageCount(value.input),
          output: usageCount(value.output),
          reasoning: usageCount(value.reasoning),
          cacheRead: usageCount(value.cacheRead),
          cacheWrite: usageCount(value.cacheWrite),
        }
        const cost = pricing ? responseCostUsd(step, model, pricing) : null
        if (cost === null) complete = false
        else {
          costUsd += cost
          pricedResponses++
        }
      }
    }
    const outputValues = usages.map(usage => {
      const output = usageCount(usage.output),
        reasoning = usageCount(usage.reasoning)
      return output === undefined || reasoning === undefined
        ? undefined
        : output + reasoning
    })
    return {
      model,
      name: pricing?.api.models[model]?.name ?? model,
      attempts: attempts.length,
      medianSeconds:
        validTimes.length === times.length ? median(validTimes) : null,
      toolCalls: sum(attempts.map(attempt => attempt.toolCalls)),
      failedTools: sum(attempts.map(attempt => attempt.failedToolCalls)),
      inputTokens: sum(usages.map(usage => usage.input)),
      outputTokens: sum(outputValues),
      cacheReadTokens: sum(usages.map(usage => usage.cacheRead)),
      cacheWriteTokens: sum(usages.map(usage => usage.cacheWrite)),
      pricedResponses,
      estimateCoverage: !pricedResponses
        ? 'unavailable'
        : complete
          ? 'complete'
          : 'partial',
      apiCostUsd: pricedResponses ? costUsd : null,
      apiCostCad:
        pricedResponses && pricing
          ? costUsd * pricing.exchange.cadPerUsd
          : null,
    }
  })
}

export function modelSummaryTable(rows: unknown[], pricing?: PricingSnapshot) {
  const counts = (value: number | null) =>
    value === null ? 'N/A' : value.toLocaleString('en-CA')
  const lines = [
    'Totals cover all recorded task attempts per model. Time includes model/session startup and tools.',
    '',
    '| Model | Median seconds/task | Tool calls | Recorded failed tools | Input tokens | Output tokens (including reasoning) | Cache-read tokens | Estimated API cost (CAD) |',
    '| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |',
  ]
  for (const summary of modelSummaries(rows, pricing)) {
    const cost =
      summary.apiCostCad === null
        ? 'N/A'
        : 'C$' +
          summary.apiCostCad.toFixed(2) +
          (summary.estimateCoverage === 'partial' ? ' (partial)' : '')
    lines.push(
      '| ' +
        [
          summary.name,
          summary.medianSeconds?.toFixed(2) ?? 'N/A',
          counts(summary.toolCalls),
          counts(summary.failedTools),
          counts(summary.inputTokens),
          counts(summary.outputTokens),
          counts(summary.cacheReadTokens),
          cost,
        ].join(' | ') +
        ' |'
    )
  }
  lines.push(
    '',
    'API-price estimates include recorded cache writes and reasoning. Missing usage or prices produce N/A or a partial estimate.',
    'Recorded failed tools count explicit tool errors and nonzero exit codes; hidden failures require transcript inspection.'
  )
  if (pricing)
    lines.push(
      `Standard API prices verified ${pricing.api.verifiedOn}: ${pricing.api.source}.`,
      `Bank of Canada rate dated ${pricing.exchange.date}: 1 USD = ${pricing.exchange.cadPerUsd} CAD. Pricing snapshot captured ${pricing.capturedAt}.`
    )
  return lines.join('\n')
}
