import fs from 'node:fs'
import path from 'node:path'
import { readJson, record, saveJson, SUITE } from './runtime.ts'

export const EXCHANGE_SOURCE =
  'https://www.bankofcanada.ca/valet/observations/FXUSDCAD?recent=1'

type Rates = {
  input: number
  cacheRead: number
  cacheWrite: number
  output: number
}
type ModelPrices = {
  name: string
  short: Rates
  long: Rates
  source?: string
  verifiedOn?: string
  shortContextMaximum?: number
  timePricing?: 'deepseek-peak-utc'
}
export type PricingSnapshot = {
  capturedAt: string
  api: {
    source: string
    verifiedOn: string
    serviceTier: 'standard'
    currency: 'USD'
    tokensPerUnit: number
    shortContextMaximum: number
    models: Record<string, ModelPrices>
  }
  exchange: { source: string; date: string; cadPerUsd: number }
}

export type UsageStep = {
  // Native assistant-response timestamp, Unix milliseconds; not eval wall time.
  timestamp?: number
  input?: number
  output?: number
  reasoning?: number
  cacheRead?: number
  cacheWrite?: number
}

export const usageCount = (value: unknown) =>
  typeof value === 'number' && Number.isSafeInteger(value) && value >= 0
    ? value
    : undefined

export function usageStep(tokens: Record<string, unknown>): UsageStep {
  const cache = tokens.cache === undefined ? {} : record(tokens.cache)
  return {
    input: usageCount(tokens.input),
    output: usageCount(tokens.output),
    reasoning: usageCount(tokens.reasoning),
    cacheRead: usageCount(cache.read),
    cacheWrite: usageCount(cache.write),
  }
}

function text(value: unknown) {
  if (typeof value !== 'string' || !value)
    throw new Error('Missing pricing text')
  return value
}

function positive(value: unknown) {
  if (typeof value !== 'number' || !Number.isFinite(value) || value <= 0)
    throw new Error('Invalid pricing number')
  return value
}

function rates(value: unknown): Rates {
  const source = record(value)
  const rate = (value: unknown) => {
    if (typeof value !== 'number' || !Number.isFinite(value) || value < 0)
      throw new Error('Invalid pricing number')
    return value
  }
  return {
    input: rate(source.input),
    output: rate(source.output),
    cacheRead: rate(source.cacheRead),
    cacheWrite: rate(source.cacheWrite),
  }
}

export function pricingSnapshot(value: unknown): PricingSnapshot {
  const source = record(value)
  const api = record(source.api)
  const exchange = record(source.exchange)
  if (api.serviceTier !== 'standard' || api.currency !== 'USD')
    throw new Error('Expected standard USD API prices')
  const models = Object.fromEntries(
    Object.entries(record(api.models)).map(([id, value]) => {
      const model = record(value)
      if (model.timePricing !== undefined && model.timePricing !== 'deepseek-peak-utc')
        throw new Error('Unknown time pricing')
      const timePricing: ModelPrices['timePricing'] = model.timePricing
      return [
        id,
        {
          name: text(model.name),
          short: rates(model.short),
          long: rates(model.long),
          ...(model.source === undefined ? {} : { source: text(model.source) }),
          ...(model.verifiedOn === undefined ? {} : { verifiedOn: text(model.verifiedOn) }),
          ...(model.shortContextMaximum === undefined ? {} : {
            shortContextMaximum: positive(model.shortContextMaximum),
          }),
          ...(timePricing === undefined ? {} : { timePricing }),
        },
      ]
    })
  )
  return {
    capturedAt: text(source.capturedAt),
    api: {
      source: text(api.source),
      verifiedOn: text(api.verifiedOn),
      serviceTier: 'standard',
      currency: 'USD',
      tokensPerUnit: positive(api.tokensPerUnit),
      shortContextMaximum: positive(api.shortContextMaximum),
      models,
    },
    exchange: {
      source: text(exchange.source),
      date: text(exchange.date),
      cadPerUsd: positive(exchange.cadPerUsd),
    },
  }
}

export async function freezePricing(
  root: string,
  suite = SUITE,
  supplied?: PricingSnapshot
) {
  const file = path.join(root, 'pricing-snapshot.json')
  if (fs.existsSync(file)) return pricingSnapshot(readJson(file))
  let snapshot = supplied
  if (!snapshot) {
    const openai = record(readJson(path.join(suite, 'api-prices.json')))
    const go = record(readJson(path.join(suite, 'go-api-prices.json')))
    if (go.currency !== openai.currency || go.tokensPerUnit !== openai.tokensPerUnit ||
      go.serviceTier !== openai.serviceTier)
      throw new Error('Incompatible provider pricing units')
    const sourcedModels = (api: Record<string, unknown>) =>
      Object.fromEntries(Object.entries(record(api.models)).map(([id, model]) => [
        id, { source: api.source, verifiedOn: api.verifiedOn, ...record(model) },
      ]))
    const openaiModels = sourcedModels(openai), goModels = sourcedModels(go)
    if (Object.keys(goModels).some(id => Object.hasOwn(openaiModels, id)))
      throw new Error('Duplicate provider/model price')
    const response = await fetch(EXCHANGE_SOURCE, {
      signal: AbortSignal.timeout(30000),
    })
    if (!response.ok)
      throw new Error('Exchange-rate lookup failed: ' + response.status)
    const body = record(await response.json())
    if (!Array.isArray(body.observations) || body.observations.length !== 1)
      throw new Error('Expected one Bank of Canada exchange-rate observation')
    const observation = record(body.observations[0])
    const rate = record(observation.FXUSDCAD).v
    if (typeof rate !== 'string' || !/^\d+(\.\d+)?$/.test(rate))
      throw new Error('Invalid Bank of Canada exchange rate')
    snapshot = pricingSnapshot({
      capturedAt: new Date().toISOString(),
      api: { ...openai, models: { ...openaiModels, ...goModels } },
      exchange: {
        source: EXCHANGE_SOURCE,
        date: observation.d,
        cadPerUsd: Number(rate),
      },
    })
  }
  const checked = pricingSnapshot(snapshot)
  saveJson(file, checked)
  return checked
}

export function responseCostUsd(
  step: UsageStep,
  model: string,
  snapshot: PricingSnapshot
) {
  const input = usageCount(step.input)
  const output = usageCount(step.output)
  const reasoning = usageCount(step.reasoning)
  const cacheRead = usageCount(step.cacheRead)
  const cacheWrite = usageCount(step.cacheWrite)
  if (
    input === undefined ||
    output === undefined ||
    reasoning === undefined ||
    cacheRead === undefined ||
    cacheWrite === undefined
  )
    return null
  const prices = snapshot.api.models[model]
  if (!prices) return null
  const context = input + cacheRead + cacheWrite
  if (
    !Number.isSafeInteger(context) ||
    !Number.isSafeInteger(output + reasoning)
  )
    return null
  const rate =
    context > (prices.shortContextMaximum ?? snapshot.api.shortContextMaximum)
      ? prices.long : prices.short
  let multiplier = 1
  if (prices.timePricing === 'deepseek-peak-utc') {
    const timestamp = usageCount(step.timestamp)
    if (timestamp === undefined) return null
    const date = new Date(timestamp)
    if (!Number.isFinite(date.getTime())) return null
    const weekday = date.getUTCDay(), hour = date.getUTCHours()
    // Start inclusive, end exclusive. Weekends are always off-peak.
    if (weekday >= 1 && weekday <= 5 &&
      ((hour >= 1 && hour < 4) || (hour >= 6 && hour < 10))) multiplier = 2
  }
  return (
    (input * rate.input +
      cacheRead * rate.cacheRead +
      cacheWrite * rate.cacheWrite +
      (output + reasoning) * rate.output) /
    snapshot.api.tokensPerUnit * multiplier
  )
}
