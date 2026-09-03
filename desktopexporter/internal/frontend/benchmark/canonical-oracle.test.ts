import { describe, expect, it } from 'vitest'
import type {
  Attribute,
  EventData,
  LinkData,
  SpanData,
  SpanNode,
  TraceData,
} from '../src/types/api-types'
import {
  TRACE_WATERFALL_SEMANTIC_FORMAT,
  TRACE_WATERFALL_SEMANTIC_MODEL_FORMAT,
  canonicalizeJSON,
  hashTraceWaterfallProjection,
  projectTraceWaterfall,
} from './canonical-oracle'

const TRACE_ID = 'trace-1'

function attribute(key: string, value: string, type = 'string'): Attribute {
  return { key, type, value }
}

function event(
  name: string,
  timestamp: bigint,
  attributes: Attribute[] = []
): EventData {
  return {
    name,
    timestamp,
    attributes,
    droppedAttributesCount: 0,
  }
}

function link(spanID: string, overrides: Partial<LinkData> = {}): LinkData {
  return {
    traceID: 'linked-trace',
    spanID,
    traceState: '',
    flags: 0,
    attributes: [],
    droppedAttributesCount: 0,
    ...overrides,
  }
}

function spanNode(
  spanOverrides: Partial<SpanData> = {},
  nodeOverrides: Partial<Omit<SpanNode, 'spanData'>> = {}
): SpanNode {
  return {
    depth: 0,
    matched: true,
    ...nodeOverrides,
    spanData: {
      traceID: TRACE_ID,
      traceState: 'vendor=opaque',
      spanID: 'span-1',
      parentSpanID: null,
      flags: 1,
      name: 'GET /orders',
      kind: 'Server',
      startTime: 9_007_199_254_740_993n,
      endTime: 9_007_199_254_741_999n,
      attributes: [],
      events: [],
      links: [],
      resource: {
        attributes: [attribute('service.name', 'checkout')],
        droppedAttributesCount: 0,
      },
      scope: {
        name: 'checkout.instrumentation',
        version: '1.2.3',
        attributes: [],
        droppedAttributesCount: 0,
      },
      droppedAttributesCount: 0,
      droppedEventsCount: 0,
      droppedLinksCount: 0,
      statusCode: 'Ok',
      statusMessage: 'healthy',
      ...spanOverrides,
    },
  }
}

function traceData(
  spans: SpanNode[] = [spanNode()],
  overrides: Partial<Omit<TraceData, 'spans'>> = {}
): TraceData {
  return {
    traceID: TRACE_ID,
    unplacedSpanCount: 0,
    ...overrides,
    spans,
  }
}

async function hashTrace(trace: TraceData): Promise<string> {
  return (await hashTraceWaterfallProjection(projectTraceWaterfall(trace))).hash
}

const KNOWN_CANONICAL_JSON =
  '{"format":"odv.trace-waterfall.semantic.v1","spans":[{"attributes":[],"cyclePoint":false,"depth":0,"droppedAttributesCount":0,"droppedEventsCount":0,"droppedLinksCount":0,"endTime":"9007199254741999","events":[],"flags":1,"kind":"Server","links":[],"matched":true,"operationName":"GET /orders","parentSpanID":null,"preorderIndex":0,"resource":{"attributes":[{"key":"service.name","type":"string","value":"checkout"}],"droppedAttributesCount":0},"salvaged":false,"scope":{"attributes":[],"droppedAttributesCount":0,"name":"checkout.instrumentation","version":"1.2.3"},"serviceName":"checkout","spanID":"span-1","startTime":"9007199254740993","status":{"code":"Ok","message":"healthy"},"traceState":"vendor=opaque"}],"traceID":"trace-1","unplacedSpanCount":0}'

describe('trace waterfall semantic oracle', () => {
  it('produces the exact known canonical JSON and SHA-256', async () => {
    const projection = projectTraceWaterfall(traceData())

    expect(projection.format).toBe(TRACE_WATERFALL_SEMANTIC_MODEL_FORMAT)
    expect(canonicalizeJSON(projection)).toBe(KNOWN_CANONICAL_JSON)
    expect(await hashTraceWaterfallProjection(projection)).toEqual({
      format: TRACE_WATERFALL_SEMANTIC_FORMAT,
      hash: '1e4426ef09bacecd4610d95bf91de86aeb8d63c3ea39ea142d43f36f5b499d36',
    })
  })

  it('changes the hash for scalar semantic changes', async () => {
    const traces = [
      traceData(),
      traceData(undefined, { unplacedSpanCount: 1 }),
      traceData([spanNode({ flags: 2 })]),
      traceData([spanNode({ statusMessage: 'changed' })]),
      traceData([spanNode({}, { matched: false })]),
      traceData([
        spanNode({
          resource: {
            attributes: [attribute('service.name', 'payments')],
            droppedAttributesCount: 0,
          },
        }),
      ]),
    ]

    const hashes = await Promise.all(traces.map(hashTrace))
    expect(new Set(hashes).size).toBe(hashes.length)
  })

  it('preserves span array order and assigns zero-based preorder indexes', async () => {
    const first = spanNode({ spanID: 'first', name: 'first' })
    const second = spanNode({ spanID: 'second', name: 'second' })
    const forward = traceData([first, second])
    const reverse = traceData([second, first])

    expect(
      projectTraceWaterfall(forward).spans.map(span => [
        span.spanID,
        span.preorderIndex,
      ])
    ).toEqual([
      ['first', 0],
      ['second', 1],
    ])
    expect(await hashTrace(forward)).not.toBe(await hashTrace(reverse))
  })

  it('sorts every visible attribute array by its UTF-8 tuple without mutation', async () => {
    const attributes = [
      attribute('same', 'z', 'string'),
      attribute('é', 'composed'),
      attribute('same', 'a', 'string'),
      attribute('é', 'decomposed'),
      attribute('same', 'a', 'bool'),
    ]
    const reversed = [...attributes].reverse()
    const withOrder = (ordered: Attribute[]) =>
      traceData([
        spanNode({
          attributes: ordered,
          events: [event('event', 9_007_199_254_741_000n, ordered)],
          links: [link('linked', { attributes: ordered })],
          resource: {
            attributes: [attribute('service.name', 'checkout'), ...ordered],
            droppedAttributesCount: 1,
          },
          scope: {
            name: 'scope',
            version: 'v1',
            attributes: ordered,
            droppedAttributesCount: 2,
          },
        }),
      ])

    const projected = projectTraceWaterfall(withOrder(attributes))
    expect(
      projected.spans[0]!.attributes.map(({ key, type, value }) => [
        key,
        type,
        value,
      ])
    ).toEqual([
      ['é', 'string', 'decomposed'],
      ['same', 'bool', 'a'],
      ['same', 'string', 'a'],
      ['same', 'string', 'z'],
      ['é', 'string', 'composed'],
    ])
    expect(
      attributes.map(({ key, type, value }) => [key, type, value])
    ).toEqual([
      ['same', 'string', 'z'],
      ['é', 'string', 'composed'],
      ['same', 'string', 'a'],
      ['é', 'string', 'decomposed'],
      ['same', 'bool', 'a'],
    ])
    expect(await hashTrace(withOrder(attributes))).toBe(
      await hashTrace(withOrder(reversed))
    )
  })

  it('treats links as a canonical multiset and preserves duplicates', async () => {
    const first = link('a', {
      traceState: 'one',
      attributes: [attribute('z', 'last'), attribute('a', 'first')],
    })
    const second = link('b', { flags: 3, droppedAttributesCount: 2 })
    const duplicate = link('a', {
      traceState: 'one',
      attributes: [attribute('a', 'first'), attribute('z', 'last')],
    })
    const forward = traceData([spanNode({ links: [first, second, duplicate] })])
    const permuted = traceData([
      spanNode({ links: [duplicate, first, second] }),
    ])

    const projectedLinks = projectTraceWaterfall(forward).spans[0]!.links
    expect(projectedLinks).toHaveLength(3)
    expect(
      projectedLinks.filter(candidate => candidate.spanID === 'a')
    ).toHaveLength(2)
    expect(await hashTrace(forward)).toBe(await hashTrace(permuted))
    expect(await hashTrace(forward)).not.toBe(
      await hashTrace(traceData([spanNode({ links: [first, second] })]))
    )
  })

  it('keeps timestamp order and canonicalizes only equal-time event groups', async () => {
    const early = event('early', 10n)
    const alpha = event('alpha', 20n, [attribute('b', '2')])
    const omega = event('omega', 20n, [attribute('a', '1')])
    const firstOrder = traceData([spanNode({ events: [early, omega, alpha] })])
    const secondOrder = traceData([spanNode({ events: [early, alpha, omega] })])

    expect(
      projectTraceWaterfall(firstOrder).spans[0]!.events.map(candidate => [
        candidate.timestamp,
        candidate.name,
      ])
    ).toEqual([
      ['10', 'early'],
      ['20', 'omega'],
      ['20', 'alpha'],
    ])
    expect(await hashTrace(firstOrder)).toBe(await hashTrace(secondOrder))
    expect(() =>
      projectTraceWaterfall(
        traceData([
          spanNode({ events: [event('later', 2n), event('earlier', 1n)] }),
        ])
      )
    ).toThrow(/timestamps must be nondecreasing/)
  })

  it('retains bigint precision as decimal strings', async () => {
    const precise = traceData([
      spanNode({
        startTime: 18_446_744_073_709_551_614n,
        endTime: 18_446_744_073_709_551_615n,
        events: [event('edge', 18_446_744_073_709_551_615n)],
      }),
    ])
    const projection = projectTraceWaterfall(precise)

    expect(projection.spans[0]!.startTime).toBe('18446744073709551614')
    expect(projection.spans[0]!.endTime).toBe('18446744073709551615')
    expect(projection.spans[0]!.events[0]!.timestamp).toBe(
      '18446744073709551615'
    )
    expect(await hashTrace(precise)).not.toBe(
      await hashTrace(
        traceData([
          spanNode({
            startTime: 18_446_744_073_709_551_613n,
            endTime: 18_446_744_073_709_551_615n,
            events: [event('edge', 18_446_744_073_709_551_615n)],
          }),
        ])
      )
    )
  })

  it('rejects a span whose end time is before its start time', () => {
    expect(() =>
      projectTraceWaterfall(
        traceData([spanNode({ startTime: 100n, endTime: 99n })])
      )
    ).toThrow(/endTime must not be before startTime/)
  })

  it('rejects a node trace ID that differs from the envelope', () => {
    expect(() =>
      projectTraceWaterfall(traceData([spanNode({ traceID: 'trace-2' })]))
    ).toThrow(/traceID must match trace\.traceID exactly/)
  })

  it.each(['resource', 'scope'] as const)(
    'requires expanded %s data',
    field => {
      const node = spanNode()
      node.spanData[field] = undefined as never
      expect(() => projectTraceWaterfall(traceData([node]))).toThrow(
        new RegExp(`${field} must exist`)
      )
    }
  )

  it('requires every cycle point to be salvaged', () => {
    expect(() =>
      projectTraceWaterfall(traceData([spanNode({}, { cyclePoint: true })]))
    ).toThrow(/cyclePoint requires salvaged/)

    expect(
      projectTraceWaterfall(
        traceData([spanNode({}, { salvaged: true, cyclePoint: true })])
      ).spans[0]
    ).toMatchObject({ salvaged: true, cyclePoint: true })
  })

  it('requires structurally valid preorder depths', () => {
    expect(() =>
      projectTraceWaterfall(traceData([spanNode({}, { depth: 1 })]))
    ).toThrow(/spans\[0\]\.depth must be 0/)
    expect(() =>
      projectTraceWaterfall(
        traceData([
          spanNode({ spanID: 'root' }),
          spanNode({ spanID: 'skipped' }, { depth: 2 }),
        ])
      )
    ).toThrow(/cannot skip a preorder level/)
  })
})

describe('numeric boundary validation', () => {
  const invalidCases: Array<[string, () => TraceData]> = [
    [
      'negative unplaced count',
      () => traceData(undefined, { unplacedSpanCount: -1 }),
    ],
    ['fractional depth', () => traceData([spanNode({}, { depth: 0.5 })])],
    ['non-finite span flags', () => traceData([spanNode({ flags: Infinity })])],
    [
      'unsafe span dropped count',
      () =>
        traceData([
          spanNode({ droppedAttributesCount: Number.MAX_SAFE_INTEGER + 1 }),
        ]),
    ],
    [
      'negative event dropped count',
      () => {
        const invalidEvent = event('bad', 1n)
        invalidEvent.droppedAttributesCount = -1
        return traceData([spanNode({ events: [invalidEvent] })])
      },
    ],
    [
      'fractional link dropped count',
      () =>
        traceData([
          spanNode({ links: [link('bad', { droppedAttributesCount: 0.5 })] }),
        ]),
    ],
    [
      'negative resource dropped count',
      () =>
        traceData([
          spanNode({
            resource: { attributes: [], droppedAttributesCount: -1 },
          }),
        ]),
    ],
    [
      'negative scope dropped count',
      () =>
        traceData([
          spanNode({
            scope: {
              name: '',
              version: '',
              attributes: [],
              droppedAttributesCount: -1,
            },
          }),
        ]),
    ],
    ['negative timestamp', () => traceData([spanNode({ startTime: -1n })])],
  ]

  it.each(invalidCases)('rejects %s', (_name, makeTrace) => {
    expect(() => projectTraceWaterfall(makeTrace())).toThrow()
  })
})

describe('canonical JSON', () => {
  it('uses JSON.stringify scalar spelling and UTF-16 object-key order', () => {
    expect(canonicalizeJSON({ '\ufffd': 1, '😀': 2, a: 1.25 })).toBe(
      '{"a":1.25,"😀":2,"�":1}'
    )
    expect(canonicalizeJSON('"\n\\\u0000')).toBe('"\\"\\n\\\\\\u0000"')
    expect(canonicalizeJSON([-0, 9_007_199_254_740_992, 1e30])).toBe(
      '[0,9007199254740992,1e+30]'
    )
  })

  it('does not Unicode-normalize strings', async () => {
    const composed = 'é'
    const decomposed = 'é'

    expect(canonicalizeJSON([composed, decomposed])).toBe('["é","é"]')
    expect(await hashTrace(traceData([spanNode({ name: composed })]))).not.toBe(
      await hashTrace(traceData([spanNode({ name: decomposed })]))
    )
  })

  const unsupportedCases: Array<[string, () => unknown]> = [
    ['undefined', () => ({ value: undefined })],
    ['non-finite number', () => ({ value: Number.NaN })],
    ['bigint', () => ({ value: 1n })],
    ['function', () => ({ value: () => undefined })],
    ['symbol', () => ({ value: Symbol('value') })],
    ['non-plain object', () => ({ value: new Date(0) })],
    ['sparse array', () => Array(1)],
    ['lone surrogate string', () => '\ud800'],
    ['lone surrogate object key', () => ({ ['bad\udc00']: true })],
    [
      'cycle',
      () => {
        const value: { self?: unknown } = {}
        value.self = value
        return value
      },
    ],
  ]

  it.each(unsupportedCases)('rejects %s', (_name, makeValue) => {
    expect(() => canonicalizeJSON(makeValue())).toThrow(
      /Unsupported canonical JSON value/
    )
  })
})
