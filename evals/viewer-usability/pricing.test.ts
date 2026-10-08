import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import apiPrices from './api-prices.json' with { type: 'json' }
import {
  freezePricing,
  pricingSnapshot,
  responseCostUsd,
  usageStep,
} from './pricing.ts'
import { TEMPORARY, readJson } from './runtime.ts'

const pricing = pricingSnapshot({
  capturedAt: '2026-10-08T06:00:00Z',
  api: apiPrices,
  exchange: { source: 'synthetic-rate', date: '2026-10-07', cadPerUsd: 1.4257 },
})

test('API estimate separately prices cache reads/writes and includes reasoning once', () => {
  const step = {
    input: 1000,
    cacheRead: 2000,
    cacheWrite: 3000,
    output: 10,
    reasoning: 40,
  }
  assert.equal(responseCostUsd(step, 'openai/gpt-6.1-sol', pricing), 0.0102)
  assert.deepEqual(
    usageStep({
      input: 1000,
      output: 10,
      reasoning: 40,
      cache: { read: 2000, write: 3000 },
    }),
    step
  )
})

test('long-context rates use each response full input including its cached tokens', () => {
  const short = {
    input: 1000,
    cacheRead: 271000,
    cacheWrite: 0,
    output: 20,
    reasoning: 80,
  }
  assert.equal(responseCostUsd(short, 'openai/gpt-6.1-sol', pricing), 0.0301)
  assert.equal(
    responseCostUsd({ ...short, input: 1001 }, 'openai/gpt-6.1-sol', pricing),
    0.059704
  )
})

test('missing, invalid or unpriced usage cannot become a zero-cost estimate', () => {
  const step = {
    input: 0,
    cacheRead: 0,
    cacheWrite: 0,
    output: 0,
    reasoning: 0,
  }
  assert.equal(responseCostUsd(step, 'openai/gpt-6.1-sol', pricing), 0)
  assert.equal(responseCostUsd(step, 'unpriced', pricing), null)
  assert.equal(
    responseCostUsd(
      { ...step, reasoning: undefined },
      'openai/gpt-6.1-sol',
      pricing
    ),
    null
  )
  for (const input of [-1, NaN, Infinity, 0.5, Number.MAX_SAFE_INTEGER + 1])
    assert.equal(
      responseCostUsd({ ...step, input }, 'openai/gpt-6.1-sol', pricing),
      null
    )
  assert.equal(
    responseCostUsd(
      { ...step, input: Number.MAX_SAFE_INTEGER, cacheRead: 1 },
      'openai/gpt-6.1-sol',
      pricing
    ),
    null
  )
})

test('pricing is snapshotted once and reused without fetching or changing existing evidence', async () => {
  const root = fs.mkdtempSync(path.join(TEMPORARY, 'eval-pricing-snapshot-'))
  assert.deepEqual(await freezePricing(root, '/unused', pricing), pricing)
  const file = path.join(root, 'pricing-snapshot.json')
  const bytes = fs.readFileSync(file)
  const changedRate = {
    ...pricing,
    exchange: { ...pricing.exchange, cadPerUsd: 2 },
  }
  assert.deepEqual(await freezePricing(root, '/unused', changedRate), pricing)
  assert.deepEqual(fs.readFileSync(file), bytes)
  assert.equal(fs.statSync(file).mode & 0o777, 0o600)
  assert.deepEqual(readJson(file), pricing)
})

test('snapshot validation rejects unusable currencies, rates and price tiers', () => {
  assert.throws(
    () =>
      pricingSnapshot({
        ...pricing,
        exchange: { ...pricing.exchange, cadPerUsd: 0 },
      }),
    /pricing number/
  )
  assert.throws(
    () =>
      pricingSnapshot({ ...pricing, api: { ...pricing.api, currency: 'CAD' } }),
    /standard USD/
  )
  assert.throws(
    () =>
      pricingSnapshot({
        ...pricing,
        api: {
          ...pricing.api,
          models: { bad: { name: 'bad', short: {}, long: {} } },
        },
      }),
    /pricing number/
  )
})
