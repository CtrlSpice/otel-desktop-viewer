import test from 'node:test'
import assert from 'node:assert/strict'
import apiPrices from './api-prices.json' with { type: 'json' }
import goPrices from './go-api-prices.json' with { type: 'json' }
import { median, p95, modelSummaries, modelSummaryTable } from './model-summary.ts'
import { pricingSnapshot } from './pricing.ts'

const pricing = pricingSnapshot({
  capturedAt: '2026-10-08T06:00:00Z',
  api: apiPrices,
  exchange: { source: 'synthetic-rate', date: '2026-10-07', cadPerUsd: 1.4257 },
})
const step = {
  input: 150000,
  output: 0,
  reasoning: 0,
  cacheRead: 0,
  cacheWrite: 0,
}
const attempt = (seconds: number, model = 'openai/gpt-6.1-sol') => ({
  provider: { label: model },
  vars: { task_id: 'task' },
  success: true,
  response: {
    metadata: {
      model,
      elapsedSeconds: seconds,
      toolCalls: 2,
      failedToolCalls: 1,
      usage: step,
      usageSteps: [step],
    },
  },
})

test('model totals aggregate attempts and use median without mutating timings', () => {
  const times = [100, 3, 1, 8]
  assert.equal(median(times), 5.5)
  assert.deepEqual(times, [100, 3, 1, 8])
  assert.equal(median([]), null)
  assert.equal(median([3, 1, 8]), 3)
  const rows = [attempt(1), attempt(3), attempt(8, 'openai/gpt-6-luna')]
  const models = modelSummaries(rows, pricing)
  assert.equal(models.length, 2)
  assert.equal(models[0].attempts, 2)
  assert.equal(models[0].totalSeconds, 4)
  assert.equal(models[0].maxSeconds, 3)
  assert.equal(models[0].medianSeconds, 2)
  assert.equal(models[0].toolCalls, 4)
  assert.equal(models[0].failedTools, 2)
  assert.equal(models[0].inputTokens, 300000)
  // Both 150K requests use short pricing even though their total exceeds 272K.
  assert.equal(models[0].apiCostUsd, 0.6)
  assert.equal(models[0].apiCostCad, 0.6 * 1.4257)
  assert.equal(models[0].estimateCoverage, 'complete')
  const table = modelSummaryTable(rows, pricing)
  assert.match(
    table,
    /Sol 6\.1 \| 2\.00 \| 4 \| 2 \| 300,000 \| 0 \| 0 \| C\$0\.86/
  )
  assert.match(table, /2026-10-07: 1 USD = 1\.4257 CAD/)
})

test('continuous p95 interpolates recorded seconds without mutating input', () => {
  const times = [100, 3, 1, 8]
  assert.ok(Math.abs(p95(times)! - 86.2) < 1e-10)
  assert.deepEqual(times, [100, 3, 1, 8])
  assert.equal(p95([]), null)
  assert.equal(p95([7]), 7)
  assert.ok(Math.abs(modelSummaries([attempt(1), attempt(3)])[0].p95Seconds! - 2.9) < 1e-10)
  assert.equal(modelSummaries([attempt(1), { provider: { label: 'openai/gpt-6.1-sol' }, response: {} }])[0].p95Seconds, null)
})

test('displayed output includes reasoning and cost uses recorded per-response categories', () => {
  const usage = {
    input: 1000,
    output: 10,
    reasoning: 40,
    cacheRead: 2000,
    cacheWrite: 3000,
  }
  const row = attempt(2)
  row.response.metadata.usage = usage
  row.response.metadata.usageSteps = [usage]
  const [summary] = modelSummaries([row], pricing)
  assert.equal(summary.outputTokens, 50)
  assert.equal(summary.cacheReadTokens, 2000)
  assert.equal(summary.cacheWriteTokens, 3000)
  assert.equal(summary.apiCostUsd, 0.0102)
})

test('missing usage is unavailable and mixed coverage is explicitly partial', () => {
  const missing = {
    provider: { label: 'openai/gpt-6.1-sol' },
    response: { error: 'setup failed' },
  }
  const [summary] = modelSummaries([attempt(2), missing], pricing)
  assert.equal(summary.inputTokens, null)
  assert.equal(summary.medianSeconds, null)
  assert.equal(summary.totalSeconds, null)
  assert.equal(summary.maxSeconds, null)
  assert.equal(summary.estimateCoverage, 'partial')
  assert.match(modelSummaryTable([attempt(2), missing], pricing), /\(partial\)/)
  const [unavailable] = modelSummaries([missing], pricing)
  assert.equal(unavailable.apiCostCad, null)
  assert.equal(unavailable.estimateCoverage, 'unavailable')
  assert.match(modelSummaryTable([missing], pricing), /N\/A/)
})

test('summaries price native response timestamps individually and report missing timestamps as partial', () => {
  const timedPricing = pricingSnapshot({ ...pricing, api: goPrices })
  const model = 'opencode-go/deepseek-v4-pro'
  const row = { ...attempt(1, model), response: { metadata: {
    ...attempt(1, model).response.metadata,
    usageSteps: [
      { ...step, timestamp: Date.parse('2026-10-12T01:00:00Z') },
      { ...step, timestamp: Date.parse('2026-10-12T04:00:00Z') },
      { ...step },
    ],
  } } }
  const [summary] = modelSummaries([row], timedPricing)
  assert.equal(summary.pricedResponses, 2)
  assert.equal(summary.apiCostUsd, 0.15 * 0.66 * 3)
  assert.equal(summary.estimateCoverage, 'partial')
  assert.match(modelSummaryTable([row], timedPricing), /2026-10-14: https:\/\/opencode.ai\/v2\/docs\/console\/go\//)
  const [missing] = modelSummaries([attempt(1, model)], timedPricing)
  assert.equal(missing.estimateCoverage, 'unavailable')
  assert.equal(missing.apiCostCad, null)
})

test('summary price citations use each provider source rather than the OpenAI default', () => {
  const combined = pricingSnapshot({ ...pricing, api: { ...apiPrices, models: {
    ...apiPrices.models,
    'opencode-go/kimi-k3': {
      ...goPrices.models['opencode-go/kimi-k3'],
      source: goPrices.source, verifiedOn: goPrices.verifiedOn,
    },
  } } })
  const table = modelSummaryTable([
    attempt(1), attempt(1, 'opencode-go/kimi-k3'),
  ], combined)
  assert.match(table, /2026-10-08: https:\/\/developers\.openai\.com\/api\/docs\/pricing/)
  assert.match(table, /2026-10-14: https:\/\/opencode.ai\/v2\/docs\/console\/go\//)
  const onlyGo = modelSummaryTable([attempt(1, 'opencode-go/kimi-k3')], combined)
  assert.doesNotMatch(onlyGo, /developers\.openai\.com/)
})
