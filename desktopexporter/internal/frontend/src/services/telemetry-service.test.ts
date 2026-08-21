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

// searchSpans ships a compressed wire shape -- resource and scope as
// references into top-level maps, times as an offset plus a duration, no
// per-span traceID -- and this service is the single place it is decoded.
//
// That makes these the only tests standing between a decoding bug and every
// view silently rendering wrong data: the waterfall, the detail panel and the
// search results all read the rehydrated SpanData and none of them can tell
// that a resource was mismatched or a timestamp reconstructed wrongly.
describe('telemetryAPI.searchSpans rehydration', () => {
  const wire = {
    traceID: 'abc123',
    traceStart: '1700000000000000000',
    unplacedSpanCount: 0,
    resources: {
      '7': {
        attributes: [
          { key: 'service.name', value: 'checkout', type: 'string' },
        ],
        droppedAttributesCount: 0,
      },
      '9': {
        attributes: [
          { key: 'service.name', value: 'payments', type: 'string' },
        ],
        droppedAttributesCount: 2,
      },
    },
    scopes: {
      '3': {
        name: 'otelhttp',
        version: '1.2.0',
        attributes: [],
        droppedAttributesCount: 0,
      },
    },
    spans: [
      {
        spanData: {
          traceState: '',
          spanID: 'aaaa',
          parentSpanID: null,
          name: 'root',
          kind: 'Server',
          start: 0,
          dur: 5_000_000,
          attributes: [],
          events: [
            {
              name: 'e',
              timestamp: '1700000000000000123',
              droppedAttributesCount: 0,
              attributes: [],
            },
          ],
          links: [],
          r: 7,
          s: 3,
          droppedAttributesCount: 0,
          droppedEventsCount: 0,
          droppedLinksCount: 0,
          statusCode: 'Ok',
          statusMessage: '',
        },
        depth: 0,
        matched: true,
      },
      {
        spanData: {
          traceState: '',
          spanID: 'bbbb',
          parentSpanID: 'aaaa',
          name: 'child',
          kind: 'Internal',
          start: 1_200_000_000,
          dur: 3_000_000,
          attributes: [],
          events: [],
          links: [],
          r: 9,
          s: 3,
          droppedAttributesCount: 0,
          droppedEventsCount: 0,
          droppedLinksCount: 0,
          statusCode: 'Ok',
          statusMessage: '',
        },
        depth: 1,
        matched: true,
      },
    ],
  }

  async function fetchTrace() {
    stubRpcResponse({ jsonrpc: '2.0', id: 1, result: wire })
    return telemetryAPI.searchSpans('abc123')
  }

  it('resolves each span against its own resource, not the first one', async () => {
    const trace = await fetchTrace()
    // Two spans, two different resources -- a decoder that ignored `r` would
    // still look plausible if every span shared one.
    expect(trace.spans[0].spanData.resource.attributes[0].value).toBe(
      'checkout'
    )
    expect(trace.spans[1].spanData.resource.attributes[0].value).toBe(
      'payments'
    )
    expect(trace.spans[1].spanData.resource.droppedAttributesCount).toBe(2)
  })

  it('resolves scopes by reference', async () => {
    const trace = await fetchTrace()
    expect(trace.spans[0].spanData.scope.name).toBe('otelhttp')
    expect(trace.spans[1].spanData.scope.name).toBe('otelhttp')
  })

  it('reconstructs absolute times from the baseline, offset and duration', async () => {
    const trace = await fetchTrace()
    const base = 1700000000000000000n

    expect(trace.spans[0].spanData.startTime).toBe(base)
    expect(trace.spans[0].spanData.endTime).toBe(base + 5_000_000n)

    // The child starts 1.2s in and lasts 3ms: end is start + dur, not
    // baseline + dur, which is the mistake the shape invites.
    expect(trace.spans[1].spanData.startTime).toBe(base + 1_200_000_000n)
    expect(trace.spans[1].spanData.endTime).toBe(base + 1_203_000_000n)
  })

  it('reattaches the traceID the wire format drops per span', async () => {
    const trace = await fetchTrace()
    expect(trace.traceID).toBe('abc123')
    for (const span of trace.spans) {
      expect(span.spanData.traceID).toBe('abc123')
    }
  })

  it('leaves no wire-only fields on the decoded span', async () => {
    const trace = await fetchTrace()
    const keys = Object.keys(trace.spans[0].spanData)
    for (const wireOnly of ['r', 's', 'start', 'dur']) {
      expect(keys).not.toContain(wireOnly)
    }
  })

  it('shares resolved resources rather than copying them per span', async () => {
    stubRpcResponse({
      jsonrpc: '2.0',
      id: 1,
      result: {
        ...wire,
        spans: [wire.spans[0], { ...wire.spans[0], depth: 1 }],
      },
    })
    const trace = await telemetryAPI.searchSpans('abc123')
    // Copying would rebuild client-side the duplication the wire format
    // exists to remove.
    expect(trace.spans[0].spanData.resource).toBe(
      trace.spans[1].spanData.resource
    )
  })

  it('still promotes event timestamps to bigint', async () => {
    const trace = await fetchTrace()
    expect(trace.spans[0].spanData.events[0].timestamp).toBe(
      1700000000000000123n
    )
  })

  // unplacedSpanCount and the per-span salvaged/cyclePoint flags are what the
  // UI reads to warn about a trace with a broken parent chain. A decoder that
  // dropped or defaulted these wrongly would make a malformed trace look
  // healthy, or an ordinary trace look broken.
  it('preserves unplacedSpanCount when the wire reports zero', async () => {
    const trace = await fetchTrace()
    expect(trace.unplacedSpanCount).toBe(0)
  })

  it('preserves unplacedSpanCount when the wire reports spans stranded on a cycle', async () => {
    stubRpcResponse({
      jsonrpc: '2.0',
      id: 1,
      result: { ...wire, unplacedSpanCount: 3 },
    })
    const trace = await telemetryAPI.searchSpans('abc123')
    expect(trace.unplacedSpanCount).toBe(3)
  })

  it('defaults unplacedSpanCount to 0 when an older store omits the field', async () => {
    const { unplacedSpanCount: _drop, ...wireWithoutField } = wire
    stubRpcResponse({
      jsonrpc: '2.0',
      id: 1,
      result: wireWithoutField,
    })
    const trace = await telemetryAPI.searchSpans('abc123')
    expect(trace.unplacedSpanCount).toBe(0)
  })

  it('does not invent salvaged or cyclePoint on spans the wire never flagged', async () => {
    const trace = await fetchTrace()
    for (const span of trace.spans) {
      expect('salvaged' in span).toBe(false)
      expect('cyclePoint' in span).toBe(false)
    }
  })

  it('preserves salvaged and cyclePoint on a span recovered from a cycle', async () => {
    stubRpcResponse({
      jsonrpc: '2.0',
      id: 1,
      result: {
        ...wire,
        unplacedSpanCount: 0,
        spans: [
          ...wire.spans,
          {
            spanData: {
              ...wire.spans[1].spanData,
              spanID: 'cccc',
              parentSpanID: 'dddd',
            },
            depth: 0,
            matched: true,
            salvaged: true,
            cyclePoint: false,
          },
          {
            spanData: {
              ...wire.spans[1].spanData,
              spanID: 'dddd',
              parentSpanID: 'cccc',
            },
            depth: 1,
            matched: true,
            salvaged: true,
            cyclePoint: true,
          },
        ],
      },
    })
    const trace = await telemetryAPI.searchSpans('abc123')

    const recovered = trace.spans.find(s => s.spanData.spanID === 'cccc')!
    expect(recovered.salvaged).toBe(true)
    expect(recovered.cyclePoint).toBe(false)

    const offender = trace.spans.find(s => s.spanData.spanID === 'dddd')!
    expect(offender.salvaged).toBe(true)
    expect(offender.cyclePoint).toBe(true)

    // The two healthy spans from the base fixture are untouched.
    expect('salvaged' in trace.spans[0]).toBe(false)
    expect('salvaged' in trace.spans[1]).toBe(false)
  })
})
