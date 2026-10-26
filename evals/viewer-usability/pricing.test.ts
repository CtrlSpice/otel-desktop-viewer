import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import apiPrices from './api-prices.json' with { type: 'json' }
import goPrices from './go-api-prices.json' with { type: 'json' }
import {
  freezePricing,
  pricingSnapshot,
  responseCostUsd,
  usageStep,
} from './pricing.ts'
import { TEMPORARY, readJson, SUITE } from './runtime.ts'

const pricing = pricingSnapshot({
  capturedAt: '2026-10-08T06:00:00Z',
  api: apiPrices,
  exchange: { source: 'synthetic-rate', date: '2026-10-07', cadPerUsd: 1.4257 },
})
const goPricing = pricingSnapshot({ ...pricing, api: goPrices })
const millionInput = {
  input: 1000000, output: 0, reasoning: 0, cacheRead: 0, cacheWrite: 0,
}

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

test('Go prices use exact provider/model keys and permit zero cache-write rates', () => {
  assert.equal(Object.keys(goPricing.api.models).length, 9)
  assert.equal(responseCostUsd(millionInput, 'opencode-go/kimi-k3', goPricing), 3)
  assert.equal(responseCostUsd(millionInput, 'kimi-k3', goPricing), null)
  assert.equal(responseCostUsd(millionInput, 'openai/kimi-k3', goPricing), null)
  assert.equal(responseCostUsd({ ...millionInput, input: 0, cacheWrite: 1000000 },
    'opencode-go/kimi-k3', goPricing), 0)
  const free = pricingSnapshot({ ...goPricing, api: { ...goPrices, models: {
    free: { name: 'free', short: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },
      long: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 } },
  } } })
  assert.equal(responseCostUsd({ ...millionInput, output: 10, reasoning: 40,
    cacheRead: 100, cacheWrite: 100 }, 'free', free), 0)
  for (const invalid of [-1, NaN, Infinity, '0', undefined]) {
    assert.throws(() => pricingSnapshot({ ...goPricing, api: {
      ...goPrices, models: { bad: { name: 'bad', short: {
        ...goPrices.models['opencode-go/kimi-k3'].short, cacheWrite: invalid,
      }, long: goPrices.models['opencode-go/kimi-k3'].long } },
    } }), /pricing number/)
  }
})

test('Qwen context tier counts cached tokens and overrides the OpenAI threshold', () => {
  const atBoundary = { ...millionInput, input: 1000, cacheRead: 254000, cacheWrite: 1000 }
  const short = (1000 * 0.4 + 254000 * 0.04 + 1000 * 0.5) / 1000000
  const long = (1001 * 1.2 + 254000 * 0.12 + 1000 * 1.5) / 1000000
  assert.equal(responseCostUsd(atBoundary, 'opencode-go/qwen3.7-plus', goPricing), short)
  assert.equal(responseCostUsd({ ...atBoundary, input: 1001 },
    'opencode-go/qwen3.7-plus', goPricing), long)
})

test('DeepSeek peak prices select UTC weekday windows with exact start/end boundaries', () => {
  const cases: [string, number][] = [
    ['2026-10-12T00:59:59.999Z', 1],
    ['2026-10-12T01:00:00.000Z', 2],
    ['2026-10-12T03:59:59.999Z', 2],
    ['2026-10-12T04:00:00.000Z', 1],
    ['2026-10-12T05:59:59.999Z', 1],
    ['2026-10-12T06:00:00.000Z', 2],
    ['2026-10-12T09:59:59.999Z', 2],
    ['2026-10-12T10:00:00.000Z', 1],
    ['2026-10-16T01:00:00.000Z', 2],
    ['2026-10-17T01:00:00.000Z', 1],
    ['2026-10-18T06:00:00.000Z', 1],
    ['2026-10-11T21:00:00.000-04:00', 2],
  ]
  for (const model of ['opencode-go/deepseek-v4-pro', 'opencode-go/deepseek-v4.1-flash']) {
    const base = goPricing.api.models[model].short.input
    for (const [date, multiplier] of cases)
      assert.equal(responseCostUsd({ ...millionInput, timestamp: Date.parse(date) },
        model, goPricing), base * multiplier, `${model} ${date}`)
  }
})

test('time-varying prices require valid native timestamps and price every category once', () => {
  const model = 'opencode-go/deepseek-v4-pro'
  for (const timestamp of [undefined, -1, NaN, Infinity, 0.5, Number.MAX_SAFE_INTEGER])
    assert.equal(responseCostUsd({ ...millionInput, timestamp }, model, goPricing), null)
  assert.equal(responseCostUsd({ ...millionInput, timestamp: 0 }, model, goPricing), 0.66)
  const step = { input: 1000, cacheRead: 2000, cacheWrite: 3000, output: 10, reasoning: 40 }
  const offPeak = (1000 * 0.66 + 2000 * 0.022 + 50 * 1.98) / 1000000
  assert.equal(responseCostUsd({ ...step, timestamp: Date.parse('2026-10-12T01:00:00Z') },
    model, goPricing), offPeak * 2)
  assert.equal(responseCostUsd({ ...step, timestamp: Date.parse('2026-10-12T04:00:00Z') },
    model, goPricing), offPeak)
  assert.throws(() => pricingSnapshot({ ...goPricing, api: { ...goPrices,
    models: { bad: { ...goPrices.models[model], timePricing: 'unknown' } },
  } }), /Unknown time pricing/)
})

test('freezing combines provider prices without changing original OpenAI sources or fetching again', async t => {
  const root = fs.mkdtempSync(path.join(TEMPORARY, 'eval-go-pricing-'))
  const fetchMock = t.mock.method(globalThis, 'fetch', async () => new Response(JSON.stringify({
    observations: [{ d: '2026-10-13', FXUSDCAD: { v: '1.4' } }],
  })))
  const frozen = await freezePricing(root, SUITE)
  assert.equal(fetchMock.mock.callCount(), 1)
  assert.equal(Object.keys(frozen.api.models).length, 15)
  for (const [id, model] of Object.entries(apiPrices.models))
    assert.deepEqual(frozen.api.models[id], {
      ...model, source: apiPrices.source, verifiedOn: apiPrices.verifiedOn,
    })
  for (const [id, model] of Object.entries(goPrices.models))
    assert.deepEqual(frozen.api.models[id], {
      ...model, source: goPrices.source, verifiedOn: goPrices.verifiedOn,
    })
  assert.deepEqual(await freezePricing(root, '/unused'), frozen)
  assert.equal(fetchMock.mock.callCount(), 1)
})
