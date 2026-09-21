import { describe, expect, it } from 'vitest'
import type { EventData, SpanNode, TraceLogSummary } from '@/types/api-types'
import {
  BAR_LABEL_GAP_REM,
  clusterTimelineRecords,
  DURATION_GUTTER_REM,
  MARKER_CLUSTER_DISTANCE_REM,
  MARKER_GLYPH_REM,
  MARKER_HIT_AREA_REM,
  markerColor,
  markerComposition,
  OUTSIDE_RECORD_SLOT_REM,
  partitionTraceLogs,
  projectTimestamp,
  recordsForSpan,
  TIMELINE_LEFT_INSET_REM,
  WATERFALL_BAR_HEIGHT_REM,
} from './timeline-markers'

const CLUSTER_DISTANCE_PX = 16

function event(name: string, timestamp: bigint): EventData {
  return { name, timestamp, attributes: [], droppedAttributesCount: 0 }
}

function span(id = 'span-1'): SpanNode {
  return {
    depth: 0,
    matched: true,
    spanData: {
      traceID: 'trace-1',
      traceState: '',
      spanID: id,
      parentSpanID: null,
      flags: 0,
      name: id,
      kindCode: 1,
      kind: 'Internal',
      startTime: 0n,
      endTime: 100n,
      attributes: [],
      events: [],
      links: [],
      resource: { attributes: [], droppedAttributesCount: 0 },
      scope: {
        name: '',
        version: '',
        attributes: [],
        droppedAttributesCount: 0,
      },
      droppedAttributesCount: 0,
      droppedEventsCount: 0,
      droppedLinksCount: 0,
      statusCode: 'Ok',
      statusCodeValue: 1,
      statusMessage: '',
    },
  }
}

function log(
  id: string,
  spanID: string | null,
  timestamp: bigint,
  severityNumber = 9
): TraceLogSummary {
  return {
    id,
    spanID,
    timestamp,
    severityText: '',
    severityNumber,
    serviceName: 'svc',
    eventName: '',
    bodyPreview: id,
  }
}

describe('timeline marker projection', () => {
  it('pins the shared row/header gutter and uniform marker geometry', () => {
    expect({
      barHeight: WATERFALL_BAR_HEIGHT_REM,
      durationGutter: DURATION_GUTTER_REM,
      outsideSlot: OUTSIDE_RECORD_SLOT_REM,
      barLabelGap: BAR_LABEL_GAP_REM,
      timelineInset: TIMELINE_LEFT_INSET_REM,
      glyph: MARKER_GLYPH_REM,
      hitArea: MARKER_HIT_AREA_REM,
      clusterDistance: MARKER_CLUSTER_DISTANCE_REM,
    }).toEqual({
      barHeight: 0.875,
      durationGutter: 5,
      outsideSlot: 2,
      barLabelGap: 0.625,
      timelineInset: 1.25,
      glyph: 0.875,
      hitArea: 1.5,
      clusterDistance: 1,
    })
  })

  it('projects exact boundaries and large bigint timestamps', () => {
    const start = 9_223_372_036_854_775_000n
    const end = start + 1_000n
    expect(projectTimestamp(start, start, end, 400)).toEqual({
      placement: 'inside',
      pixel: 0,
      percent: 0,
    })
    expect(projectTimestamp(start + 500n, start, end, 400)).toEqual({
      placement: 'inside',
      pixel: 200,
      percent: 50,
    })
    expect(projectTimestamp(end, start, end, 400)).toEqual({
      placement: 'inside',
      pixel: 400,
      percent: 100,
    })
  })

  it('classifies outside records without clamping', () => {
    expect(projectTimestamp(9n, 10n, 20n, 100)).toEqual({ placement: 'before' })
    expect(projectTimestamp(21n, 10n, 20n, 100)).toEqual({ placement: 'after' })
  })

  it('collects before and after records around an inside marker in one outside slot', () => {
    const owner = span()
    owner.spanData.events = [
      event('before', -1n),
      event('inside', 50n),
      event('after', 101n),
    ]
    const outsideLog = log('outside', 'span-1', -1n)
    const markers = clusterTimelineRecords(
      recordsForSpan(owner, [outsideLog]),
      0n,
      100n,
      100,
      CLUSTER_DISTANCE_PX
    )
    expect(markers).toHaveLength(2)
    expect(markers[0]).toMatchObject({
      position: { placement: 'inside' },
      members: [{ id: 'event:span-1:1' }],
    })
    expect(markers[1]).toMatchObject({
      position: { placement: 'outside' },
      members: [
        { id: 'event:span-1:0' },
        { id: 'log:outside' },
        { id: 'event:span-1:2' },
      ],
    })
  })
})

describe('timeline marker records and clusters', () => {
  it('orders equal-time events by received index, then logs by stable ID', () => {
    const owner = span()
    owner.spanData.events = [event('first', 50n), event('second', 50n)]
    expect(
      recordsForSpan(owner, [
        log('b', 'span-1', 50n),
        log('a', 'span-1', 50n),
      ]).map(r => r.id)
    ).toEqual(['event:span-1:0', 'event:span-1:1', 'log:a', 'log:b'])
  })

  it('clusters at the pixel threshold and recomputes for width changes', () => {
    const owner = span()
    owner.spanData.events = [event('a', 10n), event('b', 20n)]
    const records = recordsForSpan(owner, [])
    expect(
      clusterTimelineRecords(records, 0n, 100n, 160, CLUSTER_DISTANCE_PX)
    ).toHaveLength(1)
    expect(
      clusterTimelineRecords(records, 0n, 100n, 170, CLUSTER_DISTANCE_PX)
    ).toHaveLength(2)
  })

  it('identifies event, log, and mixed singleton/cluster compositions', () => {
    const owner = span()
    owner.spanData.events = [event('a', 10n), event('b', 11n)]
    const events = recordsForSpan(owner, [])
    const logRecord = recordsForSpan(span(), [log('a', 'span-1', 10n)])[0]!
    expect(
      markerComposition(
        clusterTimelineRecords(
          [events[0]!],
          0n,
          100n,
          100,
          CLUSTER_DISTANCE_PX
        )[0]!
      )
    ).toBe('event')
    expect(
      markerComposition(
        clusterTimelineRecords(
          [logRecord],
          0n,
          100n,
          100,
          CLUSTER_DISTANCE_PX
        )[0]!
      )
    ).toBe('log')
    expect(
      markerComposition(
        clusterTimelineRecords(
          [events[0]!, logRecord],
          0n,
          100n,
          100,
          CLUSTER_DISTANCE_PX
        )[0]!
      )
    ).toBe('mixed')
  })
})

describe('timeline marker ownership and colour', () => {
  it('keeps null and dangling span IDs unmatched', () => {
    const result = partitionTraceLogs(
      [
        log('owned', 'span-1', 1n),
        log('null', null, 2n),
        log('dangling', 'missing', 3n),
      ],
      [span()]
    )
    expect(result.bySpanID.get('span-1')?.map(item => item.id)).toEqual([
      'owned',
    ])
    expect(result.unmatched.map(item => item.id)).toEqual(['null', 'dangling'])
  })

  it('uses highest received log severity and event-only fallbacks', () => {
    const owner = span()
    owner.spanData.events = [event('exception', 10n)]
    const exceptionMarker = clusterTimelineRecords(
      recordsForSpan(owner, []),
      0n,
      100n,
      100,
      CLUSTER_DISTANCE_PX
    )[0]!
    expect(markerColor(exceptionMarker, '#abc')).toBe('var(--color-error)')
    owner.spanData.events = [event('normal', 10n)]
    expect(
      markerColor(
        clusterTimelineRecords(
          recordsForSpan(owner, []),
          0n,
          100n,
          100,
          CLUSTER_DISTANCE_PX
        )[0]!,
        '#abc'
      )
    ).toBe('#abc')
    const logs = recordsForSpan(span(), [
      log('warn', 'span-1', 10n, 13),
      log('fatal', 'span-1', 11n, 24),
    ])
    expect(
      markerColor(
        clusterTimelineRecords(logs, 0n, 100n, 100, CLUSTER_DISTANCE_PX)[0]!,
        '#abc'
      )
    ).toBe('var(--color-error)')
  })
})
