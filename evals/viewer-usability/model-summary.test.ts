import test from 'node:test'
import assert from 'node:assert/strict'
import apiPrices from './api-prices.json' with { type: 'json' }
import { median, modelSummaries, modelSummaryTable } from './model-summary.ts'
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
  assert.equal(summary.estimateCoverage, 'partial')
  assert.match(modelSummaryTable([attempt(2), missing], pricing), /\(partial\)/)
  const [unavailable] = modelSummaries([missing], pricing)
  assert.equal(unavailable.apiCostCad, null)
  assert.equal(unavailable.estimateCoverage, 'unavailable')
  assert.match(modelSummaryTable([missing], pricing), /N\/A/)
})
