import { describe, expect, it, vi } from 'vitest'
import { buildArmCTraceData, decodeArmCFlatTrace } from './arm-c'
import { ARM_C_FLAT_FORMAT } from './api-types'

const TRACE_ID = '574154455246414c4c00000000000001'
const LINK_TRACE_ID = '574154455246414c4c000000000000ff'
const MISSING_PARENT = '00000000000003e7'
const BASE_TIME = 1_750_000_000_000_000_000n

function spanID(index: number): string {
  return index.toString(16).padStart(16, '0')
}

function wireRow(id: string, parentSpanID: string | null, startOffset: number) {
  const start = BASE_TIME + BigInt(startOffset)
  return {
    spanID: id,
    parentSpanID,
    traceState: 'vendor=value',
    flags: 1,
    name: `span-${id}`,
    kind: 'SPAN_KIND_INTERNAL',
    startTime: start.toString(),
    endTime: (start + 10n).toString(),
    attributes: [{ key: 'span.attr', type: 'Str', value: id }],
    events: [
      {
        name: 'event',
        timestamp: (start + 1n).toString(),
        attributes: [{ key: 'event.attr', type: 'Int', value: '7' }],
        droppedAttributesCount: 2,
      },
    ],
    links: [
      {
        traceID: LINK_TRACE_ID,
        spanID: '00000000000000ff',
        traceState: '',
        flags: 1,
        attributes: [{ key: 'link.attr', type: 'Bool', value: 'true' }],
        droppedAttributesCount: 3,
      },
    ],
    resourceRef: '1',
    scopeRef: '1',
    droppedAttributesCount: 4,
    droppedEventsCount: 5,
    droppedLinksCount: 6,
    statusCode: 'STATUS_CODE_UNSET',
    statusMessage: '',
  }
}

function flatWire(rows: unknown[]) {
  return {
    format: ARM_C_FLAT_FORMAT,
    traceID: TRACE_ID,
    resources: {
      '1': {
        attributes: [
          { key: 'service.name', type: 'Str', value: 'benchmark-service' },
        ],
        droppedAttributesCount: 7,
      },
    },
    scopes: {
      '1': {
        name: 'benchmark-scope',
        version: '1.0.0',
        attributes: [{ key: 'scope.attr', type: 'Str', value: 'scope' }],
        droppedAttributesCount: 8,
      },
    },
    rows,
  }
}

function build(rows: ReturnType<typeof wireRow>[]) {
  return buildArmCTraceData(decodeArmCFlatTrace(flatWire(rows), TRACE_ID))
}

describe('Arm C flat-wire decoder', () => {
  it('rehydrates every semantic field without losing bigint precision', () => {
    const first = wireRow(spanID(1), null, 1)
    const second = wireRow(spanID(2), spanID(1), 2)
    const decoded = decodeArmCFlatTrace(flatWire([first, second]), TRACE_ID)

    expect(decoded.traceID).toBe(TRACE_ID)
    expect(decoded.spans).toHaveLength(2)
    expect(decoded.spans[0]).toMatchObject({
      traceID: TRACE_ID,
      spanID: spanID(1),
      parentSpanID: null,
      flags: 1,
      traceState: 'vendor=value',
      droppedAttributesCount: 4,
      droppedEventsCount: 5,
      droppedLinksCount: 6,
      statusCode: 'STATUS_CODE_UNSET',
    })
    expect(decoded.spans[0]!.startTime).toBe(BASE_TIME + 1n)
    expect(decoded.spans[0]!.endTime).toBe(BASE_TIME + 11n)
    expect(decoded.spans[0]!.events[0]!.timestamp).toBe(BASE_TIME + 2n)
    expect(decoded.spans[0]!.links[0]).toMatchObject({
      traceID: LINK_TRACE_ID,
      spanID: '00000000000000ff',
      flags: 1,
      droppedAttributesCount: 3,
    })
    expect(decoded.spans[0]!.resource).toBe(decoded.spans[1]!.resource)
    expect(decoded.spans[0]!.scope).toBe(decoded.spans[1]!.scope)
  })

  it('rejects unknown fields and tree metadata', () => {
    const row = { ...wireRow(spanID(1), null, 1), depth: 0 }
    expect(() => decodeArmCFlatTrace(flatWire([row]))).toThrow(
      /must contain exactly/
    )

    const wire = { ...flatWire([wireRow(spanID(1), null, 1)]), elapsed: 3 }
    expect(() => decodeArmCFlatTrace(wire)).toThrow(/must contain exactly/)
  })

  it('rejects malformed timestamps, references, IDs, and event order', () => {
    const numericTime = {
      ...wireRow(spanID(1), null, 1),
      startTime: Number(BASE_TIME),
    }
    expect(() => decodeArmCFlatTrace(flatWire([numericTime]))).toThrow(
      /startTime must be a string/
    )

    const missingResource = {
      ...wireRow(spanID(1), null, 1),
      resourceRef: '99',
    }
    expect(() => decodeArmCFlatTrace(flatWire([missingResource]))).toThrow(
      /resourceRef is not defined/
    )

    const badID = { ...wireRow(spanID(1), null, 1), spanID: 'ABC' }
    expect(() => decodeArmCFlatTrace(flatWire([badID]))).toThrow(
      /lowercase hexadecimal bytes/
    )

    const reversedEvents = wireRow(spanID(1), null, 1)
    reversedEvents.events = [
      { ...reversedEvents.events[0]!, timestamp: (BASE_TIME + 3n).toString() },
      { ...reversedEvents.events[0]!, timestamp: (BASE_TIME + 2n).toString() },
    ]
    expect(() => decodeArmCFlatTrace(flatWire([reversedEvents]))).toThrow(
      /timestamps must be nondecreasing/
    )
  })

  it('rejects a mismatched trace and an end before its start', () => {
    expect(() =>
      decodeArmCFlatTrace(
        flatWire([wireRow(spanID(1), null, 1)]),
        '574154455246414c4c00000000000002'
      )
    ).toThrow(/does not match requested/)

    const backwards = {
      ...wireRow(spanID(1), null, 10),
      endTime: BASE_TIME.toString(),
    }
    expect(() => decodeArmCFlatTrace(flatWire([backwards]))).toThrow(
      /must not precede startTime/
    )
  })
})

describe('Arm C browser tree builder', () => {
  it('builds input-order-independent healthy preorder with roots before orphans', () => {
    const rows = [
      wireRow(spanID(1), null, 30),
      wireRow(spanID(2), spanID(1), 50),
      wireRow(spanID(3), spanID(1), 40),
      wireRow(spanID(4), null, 10),
      wireRow(spanID(5), MISSING_PARENT, 1),
      wireRow(spanID(6), spanID(5), 2),
    ]
    const forward = build(rows)
    const reversed = build([...rows].reverse())

    expect(forward).toEqual(reversed)
    expect(forward.spans.map(node => node.spanData.spanID)).toEqual([
      spanID(4),
      spanID(1),
      spanID(3),
      spanID(2),
      spanID(5),
      spanID(6),
    ])
    expect(forward.spans.map(node => node.depth)).toEqual([0, 0, 1, 1, 0, 1])
    expect(forward.spans.every(node => node.matched)).toBe(true)
    expect(forward.spans.every(node => node.salvaged === undefined)).toBe(true)
    expect(forward.spans[4]!.spanData.parentSpanID).toBe(MISSING_PARENT)
    expect(forward.unplacedSpanCount).toBe(0)
  })

  it('salvages a cycle from the earliest start and marks one cycle point', () => {
    const rows = [
      wireRow(spanID(1), spanID(3), 30),
      wireRow(spanID(2), spanID(1), 10),
      wireRow(spanID(3), spanID(2), 20),
    ]
    const trace = build([rows[2]!, rows[0]!, rows[1]!])

    expect(trace.spans.map(node => node.spanData.spanID)).toEqual([
      spanID(2),
      spanID(3),
      spanID(1),
    ])
    expect(trace.spans.map(node => node.depth)).toEqual([0, 1, 2])
    expect(trace.spans.map(node => node.salvaged)).toEqual([true, true, true])
    expect(trace.spans.map(node => node.cyclePoint)).toEqual([
      true,
      false,
      false,
    ])
    expect(trace.unplacedSpanCount).toBe(0)
  })

  it('salvages independent and self-referential cycles without looping', () => {
    const rows = [
      wireRow(spanID(1), spanID(2), 10),
      wireRow(spanID(2), spanID(1), 20),
      wireRow(spanID(3), spanID(4), 30),
      wireRow(spanID(4), spanID(3), 40),
      wireRow(spanID(5), spanID(5), 50),
    ]
    const trace = build(rows.reverse())

    expect(trace.spans).toHaveLength(5)
    expect(trace.spans.filter(node => node.depth === 0)).toHaveLength(3)
    expect(trace.spans.filter(node => node.cyclePoint)).toHaveLength(3)
    expect(trace.spans.every(node => node.salvaged)).toBe(true)
    expect(trace.unplacedSpanCount).toBe(0)
  })

  it('orders equal-time salvage seeds by lowercase hexadecimal span ID', () => {
    const localeCompare = vi
      .spyOn(String.prototype, 'localeCompare')
      .mockImplementation(() => {
        throw new Error('locale-dependent comparison used')
      })
    try {
      const trace = build([
        wireRow(spanID(11), spanID(11), 10),
        wireRow(spanID(10), spanID(10), 10),
      ])

      expect(trace.spans.map(node => node.spanData.spanID)).toEqual([
        spanID(10),
        spanID(11),
      ])
      expect(trace.spans.every(node => node.cyclePoint)).toBe(true)
    } finally {
      localeCompare.mockRestore()
    }
  })

  it('rejects duplicate IDs and undefined equal-time sibling order', () => {
    expect(() =>
      build([wireRow(spanID(1), null, 1), wireRow(spanID(1), null, 2)])
    ).toThrow(/duplicate spanID/)

    expect(() =>
      build([
        wireRow(spanID(1), null, 1),
        wireRow(spanID(2), spanID(1), 2),
        wireRow(spanID(3), spanID(1), 2),
      ])
    ).toThrow(/ambiguous equal startTime/)
  })

  it('walks a deep trace iteratively without mutating its input', () => {
    const rows = Array.from({ length: 5_000 }, (_, index) =>
      wireRow(spanID(index + 1), index === 0 ? null : spanID(index), index)
    ).reverse()
    const wire = flatWire(rows)
    const before = structuredClone(wire)
    const trace = buildArmCTraceData(decodeArmCFlatTrace(wire, TRACE_ID))

    expect(trace.spans).toHaveLength(5_000)
    expect(trace.spans[4_999]!.depth).toBe(4_999)
    expect(trace.unplacedSpanCount).toBe(0)
    expect(wire).toEqual(before)
  })
})
