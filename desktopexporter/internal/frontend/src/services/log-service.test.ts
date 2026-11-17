import { afterEach, describe, expect, it, vi } from 'vitest'
import { telemetryAPI } from './telemetry-service'
import type { TraceLogSummary } from '@/types/api-types'
import { logSummaryTable, logDetailTable } from '@/test/arrow-fixtures'
import { stubArrowResult } from '@/test/telemetry-service-fixtures'

afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

describe('telemetryAPI.getLog', () => {
  it('revives an int64 body value as bigint', async () => {
    stubArrowResult(logDetailTable())

    const log = await telemetryAPI.getLog('log-1')

    expect(log.body).toEqual({
      kind: 'int64',
      value: 9_223_372_036_854_775_807n,
    })
    stubArrowResult(logDetailTable())
    const traceID = 'a'.repeat(32)
    const spanID = 'b'.repeat(16)
    const controller = new AbortController()
    expect(
      (await telemetryAPI.getSpanLogs(traceID, spanID, controller.signal))[0]
        .body
    ).toEqual(log.body)
    const request = JSON.parse(String(vi.mocked(fetch).mock.calls[0][1]!.body))
    expect(vi.mocked(fetch).mock.calls[0][0]).toBe('/api/data/getSpanLogs')
    expect(request).toEqual({ traceID, spanID })
    expect(vi.mocked(fetch).mock.calls[0][1]!.signal).toBe(controller.signal)
  })
})

describe('telemetryAPI.getTraceLogSummaries', () => {
  it('revives exact timestamps and preserves nullable span IDs', async () => {
    const result: TraceLogSummary[] = [
      {
        logRef: 'log-1',
        timestamp: 9_223_372_036_854_775_807n,
        spanID: null,
        severityText: 'INFO',
        severityNumber: 9,
        serviceName: 'checkout',
        eventName: 'order.received',
        bodyPreview: 'received',
        bodyType: 'string',
      },
    ]
    stubArrowResult(logSummaryTable())

    await expect(telemetryAPI.getTraceLogSummaries('trace-1')).resolves.toEqual(
      result
    )
  })
})
