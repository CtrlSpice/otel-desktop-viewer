import type { JsonAggregateBucket } from '@/types/wire-types'

export type RootSpan = {
  serviceName: string
  name: string
}

export type TraceSummary = {
  traceID: string
  // hasRootSpan makes the orphaned-trace state explicit so callers
  // don't have to infer it from a null rootSpan.
  hasRootSpan: boolean
  rootSpan?: RootSpan
  // Wall-clock trace bounds: earliest span start and max(end) - min(start)
  // across all spans (not root-span duration).
  startTime: bigint
  durationNs: bigint | null
  spanCount: number
  errorCount: number
}

export type TraceData = {
  traceID: string
  spans: SpanNode[]
}

export type SpanNode = {
  spanData: SpanData
  depth: number
  matched: boolean
}

export type SpanData = {
  traceID: string
  traceState: string
  spanID: string
  parentSpanID: string | null

  name: string
  kind: string
  startTime: bigint
  endTime: bigint

  attributes: Attributes
  events: EventData[]
  links: LinkData[]
  resource: ResourceData
  scope: ScopeData

  droppedAttributesCount: number
  droppedEventsCount: number
  droppedLinksCount: number

  statusCode: string
  statusMessage: string
}

export type Attribute = {
  key: string
  value: string
  type: string
}

export type Attributes = Attribute[]

export type ResourceData = {
  attributes: Attributes
  droppedAttributesCount: number
}

export type ScopeData = {
  name: string
  version: string
  attributes: Attributes
  droppedAttributesCount: number
}

export type EventData = {
  name: string
  timestamp: bigint
  attributes: Attributes
  droppedAttributesCount: number
}

export type LinkData = {
  traceID: string
  spanID: string
  traceState: string
  attributes: Attributes
  droppedAttributesCount: number
}

export type LogData = {
  id: string
  timestamp: bigint
  observedTimestamp: bigint
  traceID: string | null
  spanID: string | null
  severityText: string
  severityNumber: number
  body: string
  bodyType: string
  resource: ResourceData
  scope: ScopeData
  attributes: Attributes
  droppedAttributesCount: number
  flags: number
  eventName: string
}

// LogSummary is the lightweight card-shaped projection returned by
// the searchLogs JSON-RPC method. Full LogData (with body, attributes,
// resource, scope, etc) is fetched on demand via getLog(id).
//
// `id` is a tool-minted UUID -- in the wire payload because the UI
// needs a handle for keying, selection, and the detail fetch, but it
// must never be rendered to users (logs have no source-derived id).
//
// `timestamp` is the effective time -- the source Timestamp when set,
// otherwise ObservedTimestamp. The summary doesn't carry observed
// separately; consumers that need both fall back to the detail row.
//
// `bodyPreview` is server-truncated to the first N characters.
// Full body, traceID, spanID, and bodyType are available on LogData
// (fetched on demand for the detail pane).
export type LogSummary = {
  id: string
  timestamp: bigint
  severityText: string
  severityNumber: number
  serviceName: string
  bodyPreview: string
}

// Metrics types
export type MetricType =
  'Empty' | 'Gauge' | 'Sum' | 'Histogram' | 'ExponentialHistogram'

export type Exemplar = {
  timestamp: bigint
  value: number
  filteredAttributes: Attributes
  traceID: string | null
  spanID: string | null
}

// One measurement sample. Attributes do not live here -- they belong
// to the parent MetricTimeseries, which is what makes a sample "this
// timeseries' sample" rather than just "a sample of this metric." This
// matches the OTel data model (Metric -> Timeseries -> NumberDataPoint).
//
// Anything we'd describe as "metadata about how the tool grouped this
// sample" (e.g. attributesKey) is also a timeseries-level concept and
// lives on MetricTimeseries, not here.
type BaseDataPoint = {
  id: string
  timestamp: bigint
  /** The same instant in epoch milliseconds, from the store. Charts read this
   *  rather than dividing `timestamp` per datapoint. */
  timestampMs: number
  startTime: bigint
  flags: number
  exemplars: Exemplar[]
}

export type GaugeDataPoint = BaseDataPoint & {
  metricType: 'Gauge'
  doubleValue: number | null
  intValue: number | null
  valueType: string
}

export type SumDataPoint = BaseDataPoint & {
  metricType: 'Sum'
  doubleValue: number | null
  intValue: number | null
  valueType: string
  isMonotonic: boolean
  aggregationTemporality: string
  /** Activity since the previous reading of this series, from the store.
   *  Cumulative only; null on a series' first datapoint. */
  delta?: number | null
  /** Whether the counter restarted in that interval. */
  isReset?: boolean | null
}

export type HistogramDataPoint = BaseDataPoint & {
  metricType: 'Histogram'
  count: number
  sum: number
  min: number
  max: number
  bucketCounts: number[]
  explicitBounds: number[]
  aggregationTemporality: string
  /** Quantiles computed by the store for this bucket, keyed by the quantile
   *  (`"0.5"`). Null when none were requested. Read rather than recomputed:
   *  deriving them here walked every bucket of every series once per quantile
   *  and cost seconds on the main thread. */
  quantiles?: Record<string, number | null> | null
}

export type ExponentialHistogramDataPoint = BaseDataPoint & {
  metricType: 'ExponentialHistogram'
  count: number
  sum: number
  min: number
  max: number
  scale: number
  zeroCount: number
  zeroThreshold: number
  positiveBucketOffset: number
  positiveBucketCounts: number[]
  negativeBucketOffset: number
  negativeBucketCounts: number[]
  aggregationTemporality: string
  /** Quantiles computed by the store for this bucket, keyed by the quantile
   *  (`"0.5"`). Null when none were requested. Read rather than recomputed:
   *  deriving them here walked every bucket of every series once per quantile
   *  and cost seconds on the main thread. */
  quantiles?: Record<string, number | null> | null
}

export type DataPoint =
  | GaugeDataPoint
  | SumDataPoint
  | HistogramDataPoint
  | ExponentialHistogramDataPoint

// A MetricTimeseries is one (metric, attribute-set) pair: the OTel
// SDK spec calls this a "metric point" / "timeseries" within a metric
// stream. All datapoints inside share the same `attributes` (that's
// what makes them one timeseries). `attributesKey` is the backend's
// canonical "key=value|..." identity for this attribute set -- a stable
// id the frontend uses to drive the legend, the chart's per-line
// keying, and the per-timeseries colour assignment.
//
// (Naming note: the SDK spec uses "metric stream" for the whole named
// series produced by a View -- which corresponds to our `MetricData` /
// `metric_streams` table. The per-attribute series within it is the
// "timeseries" / "metric point". We use "timeseries" everywhere in the
// type layer to avoid colliding with the spec's "metric stream".)
//
// Timeseries arrive ordered "newest activity first" (latest dp
// timestamp desc); datapoints inside a timeseries arrive
// timestamp-desc as well. Both orderings are guaranteed by the
// backend SQL.
/** One bucket of a scalar series' Sum / Average / Rate views, computed by the
 *  store. Null values mean the bucket held no samples -- not that the answer
 *  was zero. */
export type ScalarViewBucket = {
  bucketStart: bigint
  sampleCount: number
  sum: number | null
  avg: number | null
  rate: number | null
  /** Slope of the drawn rate line's segment arriving at this bucket, in
   *  rate-units per second, from the store. Null for undrawn buckets and the
   *  first drawn one. */
  slope: number | null
  hasReset: boolean
}

/** Extremes of a series' drawn rate line, computed by the store over the same
 *  sequence the chart draws -- gap zeros included, because the reader sees
 *  them. */
export type SeriesRateStats = {
  min: number
  max: number
  avg: number
}

export type MetricTimeseries = {
  /** Series id -- stable across restarts, unique per (stream, resource, labels). */
  attributesKey: string
  attributes: Attributes
  /** The resource that emitted this series; distinguishes replicas whose
   *  labels are identical. */
  resource: ResourceData
  datapoints: DataPoint[]
  /** Min / max / avg / sum over *every* datapoint in the window, computed by
   *  the store. Null for histogram series, which carry no scalar value.
   *
   *  These exist because the client cannot compute them correctly: it sees the
   *  datapoints after thinning, so an average taken there is the mean of a
   *  sample and a sum is short by the thinning factor. */
  stats: SeriesValueStats | null
  /** Per-bucket Sum / Average / Rate from the store. Null for histograms. */
  views: ScalarViewBucket[] | null
  /** Extremes of the drawn rate line; null when there is no rate to draw. */
  rateStats: SeriesRateStats | null
  /** This series' shape at list-row resolution, reduced by the store to min and
   *  max per bucket.
   *
   *  Separate from `datapoints` because it answers a separate question: what
   *  does this line look like in 128 pixels? The row used to draw the charting
   *  points -- up to 2,000 of them -- into that box, about fifteen per pixel,
   *  for every series in the panel.
   *
   *  Present for unchecked series too. Null for histograms. */
  sparkline: SparklinePoint[] | null
}

/** Both cross-series pools, as the store folded them.
 *
 *  Same bucket shape as {@link ScalarViewBucket}, deliberately: the chart
 *  projects a pool through the same function it projects a per-series view
 *  with, rather than learning a second format for the same idea.
 *
 *  `all` never narrows with the selection -- that is what makes it "all" --
 *  while `selected` follows the checkboxes and is empty when nothing is
 *  checked, which the chart draws as "All alone". */
export type ScalarAggregate = {
  selected: ScalarViewBucket[]
  all: ScalarViewBucket[]
}

/** What getMetricAggregate resolves to: one envelope serving both metric
 *  shapes, each field null on the shape it does not apply to. */
export type MetricAggregateEnvelope = {
  aggregate: JsonAggregateBucket[] | null
  scalarAggregate: ScalarAggregate | null
}

/** Per-series value statistics, computed server-side over the full window. */
export type SeriesValueStats = {
  count: number
  min: number
  max: number
  sum: number
  avg: number
}

export type MetricData = {
  id: string
  name: string
  description: string
  unit: string
  /** Stream-level type from metric_streams (getMetric only). */
  metricType?: MetricType
  /** Stream-level temporality; null for Gauge. */
  aggregationTemporality?: string | null
  /** Stream-level monotonic flag; null except Sum. */
  isMonotonic?: boolean | null
  resourceDroppedAttributesCount: number
  resource: ResourceData
  scopeName: string
  scopeVersion: string
  scopeDroppedAttributesCount: number
  scope: ScopeData
  timeseries: MetricTimeseries[]
  /** How many datapoints the window holds, which is not necessarily how many
   *  were returned. Equal until the store starts reducing what it sends. */
  datapointCount: number
  /** The window the store's reduction actually divided.
   *
   *  Bounds are the data's own extent, and are null unless the caller said its
   *  window was the absence of a choice. A chart draws this rather than
   *  re-deriving it: the timestamps in the response are bucket starts, so
   *  measuring them describes the reduction using its own output. */
  window: {
    fittedToData: boolean
    startNs: bigint | null
    endNs: bigint | null
  }
}

// Sparkline point shape used by detail charts (not the drawer summary).
export type SparklinePoint = {
  timestamp: bigint
  value: number
}

// Metric summary for sidebar cards (one row per metric stream).
export type MetricSummary = {
  id: string
  name: string
  description: string
  unit: string
  metricType: MetricType
  aggregationTemporality: string | null
  isMonotonic: boolean | null
  serviceName: string
  // Distinct attribute sets (timeseries) seen in the queried window.
  seriesCount: number
  // In-range datapoints for this metric stream.
  dataPointCount: number
  // Most recent scalar value for Gauge/Sum metrics; null for histograms.
  lastValue: number | null
  // Timestamp of the most recent in-range datapoint (nanoseconds).
  lastSeen: bigint
}

export function metricSummaryKey(s: MetricSummary): string {
  return s.id
}

// Stats types (homepage summary cards)
export type TraceStats = {
  traceCount: number
  spanCount: number
  serviceCount: number
  errorCount: number
  lastReceived: bigint | null
}

export type LogStats = {
  logCount: number
  errorCount: number
  lastReceived: bigint | null
}

export type MetricStats = {
  metricCount: number
  dataPointCount: number
  lastReceived: bigint | null
}

export type Stats = {
  traces: TraceStats
  logs: LogStats
  metrics: MetricStats
}

// Discriminated union for search results.
// `queryTree` is the parsed query that produced these results (undefined when no search active).
// The logs variant carries LogSummary[] -- the lightweight card-shaped
// projection. Full LogData for a single row is fetched on demand via
// the getLog(id) JSON-RPC method.
export type SearchResultEvent =
  | {
      signal: 'traces'
      results: TraceSummary[]
      queryTree?: unknown
      updateSeq: number
    }
  | {
      signal: 'logs'
      results: LogSummary[]
      queryTree?: unknown
      updateSeq: number
    }
  | {
      signal: 'metrics'
      results: MetricSummary[]
      queryTree?: unknown
      updateSeq: number
    }
