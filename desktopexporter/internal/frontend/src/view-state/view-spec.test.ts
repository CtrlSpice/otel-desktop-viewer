import { describe, expect, it } from 'vitest'
import protocolVectors from './testdata/view-spec-v1.json'
import {
  canonicalViewSpecV1JSON,
  normalizeViewSpecV1,
  parseViewSpecV1JSON,
  viewSpecRevisionID,
  type ViewSpecV1,
} from './view-spec'

const METRIC_ID = '10000000-0000-4000-8000-000000000001'
const SERIES_A = '10000000-0000-4000-8000-000000000101'
const SERIES_B = '10000000-0000-4000-8000-000000000102'
const DATAPOINT_ID = '10000000-0000-4000-8000-000000000201'

const timeseriesView = {
  version: 1,
  time: { kind: 'absolute', startMs: 1_000, endMs: 2_000 },
  destination: {
    signal: 'metrics',
    metricID: METRIC_ID,
    detail: {
      kind: 'timeseries',
      aggregation: 'rate',
      queryTimezone: { kind: 'iana', name: 'America/New_York' },
      selectedSeries: SERIES_B,
      selectedDatapoint: DATAPOINT_ID,
      visibleSeries: [SERIES_B, SERIES_A],
      showAllSeriesAggregate: true,
      showSelectionStatOverlays: false,
    },
  },
  list: {
    search: 'service.name = "checkout<&> 😀" LIMIT 22',
    sort: { field: 'lastSeen', direction: 'desc' },
  },
} satisfies ViewSpecV1

describe('normalizeViewSpecV1', () => {
  it('normalizes a complete metric view into schema order', () => {
    expect(normalizeViewSpecV1(timeseriesView)).toEqual({
      ...timeseriesView,
      destination: {
        ...timeseriesView.destination,
        detail: {
          ...timeseriesView.destination.detail,
          visibleSeries: [SERIES_A, SERIES_B],
        },
      },
    })
  })

  it('preserves an explicitly empty visible-series set', () => {
    const view: ViewSpecV1 = structuredClone(timeseriesView)
    if (view.destination.signal !== 'metrics' || !view.destination.detail) {
      throw new Error('expected metric fixture')
    }
    view.destination.detail.visibleSeries = []
    view.destination.detail.selectedSeries = null
    view.destination.detail.selectedDatapoint = null

    const normalized = normalizeViewSpecV1(view)
    expect(normalized?.destination.signal).toBe('metrics')
    if (normalized?.destination.signal !== 'metrics') return
    expect(normalized.destination.detail?.visibleSeries).toEqual([])
  })

  it('deduplicates and sorts set-like fields', () => {
    const histogram = {
      version: 1,
      time: { kind: 'all' },
      destination: {
        signal: 'metrics',
        metricID: METRIC_ID,
        detail: {
          kind: 'histogram',
          tab: 'quantiles',
          scope: 'window',
          queryTimezone: { kind: 'offset', utcOffsetMinutes: -300 },
          selectedSeries: null,
          selectedDatapoint: null,
          visibleSeries: [SERIES_B, SERIES_A, SERIES_B],
          activeQuantile: '0.99',
        },
      },
      list: {
        search: '',
        sort: { field: 'name', direction: 'asc' },
      },
    }

    const normalized = normalizeViewSpecV1(histogram)
    expect(normalized?.destination.signal).toBe('metrics')
    if (normalized?.destination.signal !== 'metrics') return
    expect(normalized.destination.detail).toMatchObject({
      visibleSeries: [SERIES_A, SERIES_B],
      activeQuantile: '0.99',
    })
  })

  it('canonicalizes field-specific identifiers', () => {
    const view = structuredClone(timeseriesView)
    view.destination.metricID = METRIC_ID.toUpperCase()
    view.destination.detail.selectedSeries = SERIES_B.toUpperCase()
    view.destination.detail.selectedDatapoint = DATAPOINT_ID.toUpperCase()
    view.destination.detail.visibleSeries = [
      SERIES_B.toUpperCase(),
      SERIES_A.toUpperCase(),
    ]

    expect(normalizeViewSpecV1(view)).toEqual(
      normalizeViewSpecV1(timeseriesView)
    )
  })

  it('accepts trace, log, and home destinations', () => {
    expect(
      normalizeViewSpecV1({
        version: 1,
        time: { kind: 'all' },
        destination: {
          signal: 'traces',
          traceID: '0123456789abcdef0123456789abcdef',
          spanID: '0123456789abcdef',
          eventIndex: 0,
        },
        list: {
          search: 'status = error',
          sort: { field: 'duration', direction: 'desc' },
        },
      })
    ).not.toBeNull()

    expect(
      normalizeViewSpecV1({
        version: 1,
        time: { kind: 'absolute', startMs: 10, endMs: 20 },
        destination: {
          signal: 'logs',
          logID: '20000000-0000-4000-8000-000000000001',
        },
        list: {
          search: '',
          sort: { field: 'timestamp', direction: 'desc' },
        },
      })
    ).not.toBeNull()

    expect(
      normalizeViewSpecV1({
        version: 1,
        time: { kind: 'all' },
        destination: { signal: 'home' },
      })
    ).toEqual({
      version: 1,
      time: { kind: 'all' },
      destination: { signal: 'home' },
    })
  })

  it('accepts the signed nanosecond timestamp boundary', () => {
    expect(
      normalizeViewSpecV1({
        version: 1,
        time: {
          kind: 'absolute',
          startMs: 9_223_372_036_853,
          endMs: 9_223_372_036_854,
        },
        destination: { signal: 'home' },
      })
    ).not.toBeNull()
  })

  it.each([
    {
      name: 'unknown version',
      value: { ...timeseriesView, version: 2 },
    },
    {
      name: 'backwards absolute range',
      value: {
        ...timeseriesView,
        time: { kind: 'absolute', startMs: 2, endMs: 1 },
      },
    },
    {
      name: 'timestamp beyond signed nanoseconds',
      value: {
        ...timeseriesView,
        time: {
          kind: 'absolute',
          startMs: 9_223_372_036_854,
          endMs: 9_223_372_036_855,
        },
      },
    },
    {
      name: 'unknown sort field',
      value: {
        ...timeseriesView,
        list: {
          ...timeseriesView.list,
          sort: { field: 'unknown', direction: 'asc' },
        },
      },
    },
    {
      name: 'event without span',
      value: {
        version: 1,
        time: { kind: 'all' },
        destination: {
          signal: 'traces',
          traceID: '0123456789abcdef0123456789abcdef',
          spanID: null,
          eventIndex: 0,
        },
        list: {
          search: '',
          sort: { field: 'startTime', direction: 'desc' },
        },
      },
    },
    {
      name: 'metric detail without metric',
      value: {
        ...timeseriesView,
        destination: { ...timeseriesView.destination, metricID: null },
      },
    },
    {
      name: 'unknown quantile',
      value: {
        version: 1,
        time: { kind: 'all' },
        destination: {
          signal: 'metrics',
          metricID: METRIC_ID,
          detail: {
            kind: 'histogram',
            tab: 'quantiles',
            scope: 'window',
            queryTimezone: { kind: 'iana', name: 'UTC' },
            selectedSeries: null,
            selectedDatapoint: null,
            visibleSeries: [],
            activeQuantile: '0.9',
          },
        },
        list: {
          search: '',
          sort: { field: 'name', direction: 'asc' },
        },
      },
    },
    {
      name: 'datapoint without its series',
      value: {
        ...timeseriesView,
        destination: {
          ...timeseriesView.destination,
          detail: {
            ...timeseriesView.destination.detail,
            selectedSeries: null,
          },
        },
      },
    },
    {
      name: 'selected series hidden by the exact set',
      value: {
        ...timeseriesView,
        destination: {
          ...timeseriesView.destination,
          detail: {
            ...timeseriesView.destination.detail,
            visibleSeries: [SERIES_A],
          },
        },
      },
    },
    {
      name: 'malformed metric identifier',
      value: {
        ...timeseriesView,
        destination: { ...timeseriesView.destination, metricID: 'metric-1' },
      },
    },
    {
      name: 'invalid query timezone',
      value: {
        ...timeseriesView,
        destination: {
          ...timeseriesView.destination,
          detail: {
            ...timeseriesView.destination.detail,
            queryTimezone: { kind: 'iana', name: '../../localtime' },
          },
        },
      },
    },
    {
      name: 'non-canonical Unicode search text',
      value: {
        ...timeseriesView,
        list: { ...timeseriesView.list, search: 'bad\u2028query' },
      },
    },
    {
      name: 'oversized search text',
      value: {
        ...timeseriesView,
        list: { ...timeseriesView.list, search: 'x'.repeat(65_537) },
      },
    },
  ])('rejects $name', ({ value }) => {
    expect(normalizeViewSpecV1(value)).toBeNull()
  })

  it('enforces the scalar chart visibility cap', () => {
    const series = Array.from(
      { length: 23 },
      (_, index) => `30000000-0000-4000-8000-${String(index).padStart(12, '0')}`
    )
    const view: ViewSpecV1 = structuredClone(timeseriesView)
    if (view.destination.signal !== 'metrics' || !view.destination.detail) {
      throw new Error('expected metric fixture')
    }
    view.destination.detail.selectedSeries = null
    view.destination.detail.selectedDatapoint = null
    view.destination.detail.visibleSeries = series

    expect(normalizeViewSpecV1(view)).toBeNull()
  })

  it('applies cardinality limits after deduplication', () => {
    const view = structuredClone(timeseriesView)
    view.destination.detail.visibleSeries = Array(23).fill(SERIES_B)

    expect(normalizeViewSpecV1(view)).toEqual(
      normalizeViewSpecV1({
        ...timeseriesView,
        destination: {
          ...timeseriesView.destination,
          detail: {
            ...timeseriesView.destination.detail,
            visibleSeries: [SERIES_B],
          },
        },
      })
    )
  })
})

describe('ViewSpecV1 JSON', () => {
  it('round-trips through canonical JSON', () => {
    const json = canonicalViewSpecV1JSON(timeseriesView)
    expect(parseViewSpecV1JSON(json)).toEqual(
      normalizeViewSpecV1(timeseriesView)
    )
  })

  it('drops unknown fields from the canonical form', () => {
    const json = canonicalViewSpecV1JSON({
      version: 1,
      ignored: 'root',
      time: { kind: 'all', ignored: 'time' },
      destination: { signal: 'home', ignored: 'destination' },
    })
    expect(json).toBe(
      '{"version":1,"time":{"kind":"all"},"destination":{"signal":"home"}}'
    )
  })

  it('rejects malformed JSON and invalid values', () => {
    expect(parseViewSpecV1JSON('{')).toBeNull()
    expect(() => canonicalViewSpecV1JSON({ version: 1 })).toThrow(
      'Invalid ViewSpecV1'
    )
  })
})

describe('viewSpecRevisionID', () => {
  it('is stable across semantically equivalent set ordering', async () => {
    const reordered = structuredClone(timeseriesView)
    if (
      reordered.destination.signal !== 'metrics' ||
      !reordered.destination.detail
    ) {
      throw new Error('expected metric fixture')
    }
    reordered.destination.detail.visibleSeries = [SERIES_A, SERIES_B, SERIES_A]

    await expect(viewSpecRevisionID(timeseriesView)).resolves.toBe(
      await viewSpecRevisionID(reordered)
    )
  })
})

describe('ViewSpecV1 protocol vectors', () => {
  it.each(protocolVectors)(
    'matches $name canonical bytes and ID',
    async vector => {
      expect(canonicalViewSpecV1JSON(vector.input)).toBe(vector.canonical)
      await expect(viewSpecRevisionID(vector.input)).resolves.toBe(vector.id)
    }
  )
})
