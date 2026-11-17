import type { Stats } from '@/types/api-types'
import type { JsonStats } from '@/types/wire-types'
import { callRPC } from './telemetry-request'
import { traceAPI } from './trace-service'
import { logAPI } from './log-service'
import { metricAPI } from './metric-service'
import { discoveryAPI } from './discovery-service'
import { attributeSearchAPI } from './attribute-search-service'
export { JsonRpcError } from './service-error'
export {
  RequestAbortedError,
  isAbortError,
  convertQueryTreeForBackend,
  type SearchSort,
  type QueryTimeBound,
} from './telemetry-request'

// oxlint-disable-next-line anti-slop/no-unknown-parameters -- Validates the raw wire value before bigint conversion.
function bigintFromWire(value: unknown): bigint {
  // oxlint-disable-next-line anti-slop/no-runtime-typeof -- Rejects non-string wire values before bigint conversion so rounded JSON numbers cannot be accepted as exact integers.
  if (typeof value !== 'string') {
    // oxlint-disable-next-line anti-slop/no-runtime-typeof -- Reports the rejected wire value's runtime type in the boundary validation error.
    const received = value === null ? 'null' : typeof value
    throw new Error(
      `Invalid bigint wire value: expected string, got ${received}`
    )
  }
  return BigInt(value)
}

// oxlint-disable-next-line anti-slop/no-unknown-parameters -- Accepts null; otherwise delegates to the validating wire decoder.
function nullableBigintFromWire(value: unknown): bigint | null {
  return value === null ? null : bigintFromWire(value)
}

function statsFromJSON(json: JsonStats): Stats {
  return {
    storage: json.storage,
    traces: {
      ...json.traces,
      lastReceived: nullableBigintFromWire(json.traces.lastReceived),
    },
    logs: {
      ...json.logs,
      lastReceived: nullableBigintFromWire(json.logs.lastReceived),
    },
    metrics: {
      ...json.metrics,
      lastReceived: nullableBigintFromWire(json.metrics.lastReceived),
    },
    rejections: (json.rejections ?? []).map(r => ({
      ...r,
      samples: r.samples ?? [],
      firstSeen: nullableBigintFromWire(r.firstSeen),
      lastSeen: nullableBigintFromWire(r.lastSeen),
    })),
  }
}

export let telemetryAPI = {
  ...traceAPI,
  ...logAPI,
  ...metricAPI,
  ...discoveryAPI,
  ...attributeSearchAPI,
  // Stats methods
  getImportConfig: async (
    signal?: AbortSignal
  ): Promise<{ otlpHttpPort: number; otlpGrpcPort?: number }> => {
    const config = await callRPC<{
      otlpHttpPort: number
      otlpGrpcPort?: number
    }>('getImportConfig', undefined, signal)
    if (
      !Number.isInteger(config?.otlpHttpPort) ||
      config.otlpHttpPort < 1 ||
      config.otlpHttpPort > 65535
    ) {
      throw new Error('The viewer returned an invalid OTLP HTTP port')
    }
    if (
      config.otlpGrpcPort !== undefined &&
      (!Number.isInteger(config.otlpGrpcPort) ||
        config.otlpGrpcPort < 1 ||
        config.otlpGrpcPort > 65535)
    ) {
      throw new Error('The viewer returned an invalid OTLP gRPC port')
    }
    return config
  },

  getStats: async (signal?: AbortSignal): Promise<Stats> => {
    const rawData = await callRPC<JsonStats>('getStats', undefined, signal)
    return statsFromJSON(rawData)
  },
}
