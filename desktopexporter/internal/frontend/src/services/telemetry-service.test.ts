import { afterEach, describe, expect, it, vi } from 'vitest'
import { telemetryAPI, JsonRpcError } from './telemetry-service'

// The backend signals not-found with JSON-RPC errors (one convention across
// all signals; see internal/server/errors.go). getMetric's callers expect
// MetricData | null, so the service translates exactly one code -- -32003,
// metric not found -- back to null. These tests pin that translation.

function stubRpcResponse(body: unknown) {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue({
      ok: true,
      json: async () => body,
    })
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('telemetryAPI.getMetric', () => {
  it('returns null when the backend reports metric not found (-32003)', async () => {
    stubRpcResponse({
      jsonrpc: '2.0',
      id: 1,
      error: { code: -32003, message: 'Metric not found' },
    })
    await expect(
      telemetryAPI.getMetric('some-stream', 0, 1)
    ).resolves.toBeNull()
  })

  it('rethrows JSON-RPC errors other than metric not found', async () => {
    stubRpcResponse({
      jsonrpc: '2.0',
      id: 1,
      error: { code: -32009, message: 'Invalid metric stream ID' },
    })
    const call = telemetryAPI.getMetric('not-a-stream', 0, 1)
    await expect(call).rejects.toBeInstanceOf(JsonRpcError)
    await expect(call).rejects.toMatchObject({ code: -32009 })
  })

  it('parses a successful result into MetricData', async () => {
    stubRpcResponse({
      jsonrpc: '2.0',
      id: 1,
      result: {
        name: 'test.gauge',
        unit: 'bytes',
        metricType: 'Gauge',
        timeseries: [],
      },
    })
    const metric = await telemetryAPI.getMetric('some-stream', 0, 1)
    expect(metric).not.toBeNull()
    expect(metric!.name).toBe('test.gauge')
    expect(metric!.timeseries).toEqual([])
  })
})
