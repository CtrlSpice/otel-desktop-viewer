import type { JsonAggregateBucket } from '@/types/wire-types'

export type RootSpan = {
  serviceName: string
  name: string
}

export type MatchedSpan = {
  traceID: string
  spanID: string
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
  /** Computed identities of matching spans; present only for filtered searches. */
  matchedSpans?: MatchedSpan[]
}

export type TraceData = {
  traceID: string
  /**
   * Spans in the trace that could not be placed under any root, and so are
   * missing from `spans`. Normally 0; non-zero means malformed parent links
   * (a cycle), which the UI surfaces rather than rendering a short trace.
   */
  unplacedSpanCount: number
  spans: SpanNode[]
}

export type SpanNode = {
  spanData: SpanData
  depth: number
  matched: boolean
  /** Recovered from a stranded (cyclic) part of the trace; absent normally. */
  salvaged?: true
  /** Retained cycle cut whose reported parent appears below it. */
  cyclePoint?: boolean
}

export type SpanData = {
  traceID: string
  traceState: string
  spanID: string
  parentSpanID: string | null
  /** W3C trace flags, plus whether the parent context was remote. */
  flags: number

  name: string
  /** Authoritative received OTLP SpanKind int32. */
  kindCode: number
  /** Readable label derived from kindCode by SQL. */
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

  /** Authoritative received OTLP StatusCode int32. */
  statusCodeValue: number
  /** Readable label derived from statusCodeValue by SQL. */
  statusCode: string
  statusMessage: string
}

export type Attribute = {
  id?: string
  key: string
  value: AttributeValue
  hasConflict?: boolean
}

export type AttributeValue =
  | { kind: 'empty'; value: null }
  | { kind: 'string' | 'bytes'; value: string }
  | { kind: 'bool'; value: boolean }
  | { kind: 'int64'; value: bigint }
  | { kind: 'double'; value: number }
  | { kind: 'array'; value: AttributeValue[] }
  | { kind: 'map'; value: AttributeMapEntry[]; conflictingKeys?: string[] }

export type AttributeMapEntry = {
  key: string
  value: AttributeValue
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
  traceID: string | null
  spanID: string | null
  traceState: string
  attributes: Attributes
  droppedAttributesCount: number
  /** W3C trace flags for the linked context. */
  flags: number
}

export type LogData = {
  logRef: string
  timestamp: bigint
  observedTimestamp: bigint
  traceID: string | null
  spanID: string | null
  severityText: string
  severityNumber: number
  body: AttributeValue
  resource: ResourceData
  scope: ScopeData
  attributes: Attributes
  droppedAttributesCount: number
  flags: number
  eventName: string
}

// LogSummary is the lightweight card-shaped projection returned by
// the searchLogSummaries JSON-RPC method. Full LogData (with body, attributes,
// resource, scope, etc) is fetched on demand via getLog(logRef).
//
// `logRef` is a tool-minted UUID -- in the wire payload because the UI
// needs a handle for keying, selection, and the detail fetch, but it
// must never be rendered to users (logs have no source-derived id).
//
// `timestamp` is the effective time -- the source Timestamp when set,
// otherwise ObservedTimestamp. The summary doesn't carry observed
// separately; consumers that need both fall back to the detail row.
//
// `bodyPreview` is server-truncated to the first N characters.
// Full body, traceID, and spanID are available on LogData
// (fetched on demand for the detail pane).
export type LogSummary = {
  logRef: string
  timestamp: bigint
  severityText: string
  severityNumber: number
  serviceName: string
  bodyPreview: string
}

/** Complete trace-scoped log projection used for span correlation. */
export type TraceLogSummary = LogSummary & {
  spanID: string | null
  eventName: string
}

// Metrics types
export type MetricType =
  'Empty' | 'Gauge' | 'Sum' | 'Histogram' | 'ExponentialHistogram'

type ExemplarBase = {
  timestamp: bigint
  filteredAttributes: Attributes
  traceID: string | null
  spanID: string | null
}

export type Exemplar = ExemplarBase &
  (
    | { valueType: 'Double'; doubleValue: number; intValue: null }
    | { valueType: 'Int'; doubleValue: null; intValue: bigint }
    | { valueType: 'Empty'; doubleValue: null; intValue: null }
  )

// One measurement sample. Attributes do not live here -- they belong
// to the parent MetricSeriesViewData, which is what makes a sample "this
// timeseries' sample" rather than just "a sample of this metric." This
// matches the OTel data model (Metric -> Timeseries -> NumberDataPoint).
//
// Anything we'd describe as "metadata about how the tool grouped this
// sample" (e.g. seriesRef) is also a timeseries-level concept and
// lives on MetricSeriesViewData, not here.
type BaseDataPoint = {
  id: string
  timestamp: bigint
  /** The same instant in epoch milliseconds, from the store. Charts read this
   *  rather than dividing `timestamp` per datapoint. */
  timestampMs: number
  startTime: bigint
  flags: number
  exemplars: Exemplar[]
  /** How many the datapoint holds, present only when the store's cap trimmed
   *  the list. Absent means `exemplars` is all of them. */
  exemplarCount?: number
}

export type GaugeDataPoint = BaseDataPoint & {
  metricType: 'Gauge'
  doubleValue: number | null
  /** Received NumberDataPoint.as_int, kept exact across JSON. */
  intValue: bigint | null
  valueType: string
}

export type SumDataPoint = BaseDataPoint & {
  metricType: 'Sum'
  doubleValue: number | null
  /** Received NumberDataPoint.as_int, kept exact across JSON. */
  intValue: bigint | null
  valueType: string
  isMonotonic: boolean
  /** Authoritative received OTLP AggregationTemporality int32. */
  aggregationTemporalityCode: number
  /** Readable label derived from aggregationTemporalityCode by SQL. */
  aggregationTemporality: string
  /** Activity since the previous reading of this series, from the store.
   *  Exact integral results are bigint; double-domain results are number.
   *  Cumulative only; null on a series' first datapoint. */
  delta?: number | bigint | null
  /** Whether the counter restarted in that interval. */
  isReset?: boolean | null
}

export type HistogramDataPoint = BaseDataPoint & {
  metricType: 'Histogram'
  /** Received uint64 count, or an exact integral SQL reduction of such counts. */
  count: bigint
  /** Optional received statistics on raw rows; null means the sender omitted
   *  the field. Reduced sum is null unless every input supplied it; reduced
   *  min/max are bucket-derived display values. */
  sum: number | null
  min: number | null
  max: number | null
  /** Received uint64 vector, or an exact integral SQL reduction of one. */
  bucketCounts: bigint[]
  explicitBounds: number[]
  aggregationTemporalityCode: number
  aggregationTemporality: string
  /** Store-computed quantiles keyed by value (`"0.5"`); null when not requested. */
  quantiles?: Record<string, number | null> | null
}

export type ExponentialHistogramDataPoint = BaseDataPoint & {
  metricType: 'ExponentialHistogram'
  /** Received uint64 count, or an exact integral SQL reduction of such counts. */
  count: bigint
  /** Optional received statistics on raw rows; null means the sender omitted
   *  the field. Reduced sum is null unless every input supplied it; reduced
   *  min/max are bucket-derived display values. */
  sum: number | null
  min: number | null
  max: number | null
  scale: number
  /** Received uint64 zero count, or an exact integral SQL reduction of one. */
  zeroCount: bigint
  zeroThreshold: number
  positiveBucketOffset: number
  /** Received uint64 vector, or an exact integral SQL reduction of one. */
  positiveBucketCounts: bigint[]
  negativeBucketOffset: number
  /** Received uint64 vector, or an exact integral SQL reduction of one. */
  negativeBucketCounts: bigint[]
  aggregationTemporalityCode: number
  aggregationTemporality: string
  /** Store-computed quantiles keyed by value (`"0.5"`); null when not requested. */
  quantiles?: Record<string, number | null> | null
}

export type DataPoint =
  | GaugeDataPoint
  | SumDataPoint
  | HistogramDataPoint
  | ExponentialHistogramDataPoint

// A MetricSeriesViewData is one Metric and attribute-set pair. All datapoints
// share the same attributes. `seriesRef` is its database-local reference.
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

export type MetricSeriesViewData = {
  /** Database-local series reference. */
  seriesRef: string
  attributes: Attributes
  /** Resource associated with the parent Metric. */
  resource: ResourceData
  datapoints: DataPoint[]
  /** Min / max / avg / sum over *every* datapoint in the window, computed by
   *  the store. Null for histogram series, which carry no scalar value.
   *
   *  These exist because the client cannot compute them correctly: it sees the
   *  datapoints after thinning, so an average taken there is the mean of a
   *  sample and a sum is short by the thinning factor. */
  stats: SeriesValueStats | null
  /** Datapoints the window holds for this series, from the store. Not
   *  `datapoints.length`, which is what this response carried after narrowing
   *  and reduction. */
  datapointCount: number
  /** When this series last reported in the window. */
  lastSeenNs: bigint | null
  /** Per-bucket Sum / Average / Rate from the store. Null for histograms. */
  views: ScalarViewBucket[] | null
  /** Extremes of the drawn rate line; null when there is no rate to draw. */
  rateStats: SeriesRateStats | null
  /** Store-reduced row sparkline with min and max per bucket. Present for
   *  unchecked series; null for histograms. */
  sparkline: SparklinePoint[] | null
}

/** What the store refused to merge because its inputs carried different
 *  explicit bounds. Boundary sets cannot be rescaled safely. */
export type BoundsMismatch = {
  seriesBuckets: number
  aggregateBuckets: number
}

export type ScalarAggregate = {
  /** Checked series; empty when none are checked. */
  selected: ScalarViewBucket[]
  /** Every series, independent of selection. */
  all: ScalarViewBucket[]
}

export type AggregateBucket = Omit<
  JsonAggregateBucket,
  'sum' | 'min' | 'max' | 'explicitBounds' | 'zeroThreshold' | 'quantiles'
> & {
  sum: number | null
  min?: number
  max?: number
  explicitBounds?: number[]
  zeroThreshold?: number
  quantiles: Record<string, number | null> | null
}

/** What getMetricAggregateView resolves to: one envelope serving both metric
 *  shapes, each field null on the shape it does not apply to. */
export type MetricAggregateViewData = {
  aggregate: AggregateBucket[] | null
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

type OptionalMetricTemporality =
  | {
      aggregationTemporality?: never
      aggregationTemporalityCode?: never
    }
  | {
      aggregationTemporality: string | null
      aggregationTemporalityCode: number | null
    }

export type MetricViewData = {
  /** The window's most recent datapoint across every series. */
  lastSeenNs: bigint | null
  metricRef: string
  name: string
  description: string
  /** OTLP Metric.metadata: describes the instrument, not any one series. */
  metadata: Attributes
  unit: string
  /** Metric type from metrics (getMetricView only). */
  metricType?: MetricType
  /** Metric monotonic flag; null except Sum. */
  isMonotonic?: boolean | null
  resourceDroppedAttributesCount: number
  /** Latest received ResourceMetrics schema URL for this Metric. */
  resourceSchemaUrl: string
  resource: ResourceData
  scopeName: string
  scopeVersion: string
  /** Received ScopeMetrics schema URL; part of InstrumentationScope identity. */
  scopeSchemaUrl: string
  scopeDroppedAttributesCount: number
  scope: ScopeData
  timeseries: MetricSeriesViewData[]
  /** How many datapoints the window holds, which is not necessarily how many
   *  were returned. Equal until the store starts reducing what it sends. */
  datapointCount: number
  /** Merges refused for disagreeing explicit bounds; null when none were. */
  boundsMismatch: BoundsMismatch | null
  /** Requested bounds and the bounds the store's reduction actually divided. */
  window: {
    requested: { startNs: bigint | null; endNs: bigint | null }
    effective: { startNs: bigint | null; endNs: bigint | null }
  }
} & OptionalMetricTemporality

export type ExactMetricResource = {
  attributes: Attributes
  droppedAttributesCount: number
  schemaUrl: string
}

export type ExactMetricScope = {
  name: string
  version: string
  attributes: Attributes
  droppedAttributesCount: number
  schemaUrl: string
}

type ExactMetricIdentityBase = {
  metricRef: string
  name: string
  description: string
  unit: string
  metadata: Attributes
  resource: ExactMetricResource
  scope: ExactMetricScope
}

export type ExactMetricIdentity = ExactMetricIdentityBase &
  (
    | { metricType: 'Gauge' }
    | {
        metricType: 'Sum'
        aggregationTemporalityCode: number
        isMonotonic: boolean
      }
    | {
        metricType: 'Histogram' | 'ExponentialHistogram'
        aggregationTemporalityCode: number
      }
  )

export type MetricSeriesSummary = {
  seriesRef: string
  attributes: Attributes
  /** Computed count of retained datapoints in this series. */
  datapointCount: bigint
  /** Computed earliest received datapoint timestamp, or null with no points. */
  firstDatapointTimestamp: bigint | null
  /** Computed latest received datapoint timestamp, or null with no points. */
  lastDatapointTimestamp: bigint | null
}

export type ExactMetric = ExactMetricIdentity & {
  series: MetricSeriesSummary[]
}

type ReceivedDataPointBase = {
  datapointID: string
  timestamp: bigint
  startTime: bigint
  flags: number
  exemplars: Exemplar[]
}

export type ReceivedNumberDataPoint = ReceivedDataPointBase &
  (
    | { valueType: 'Int'; intValue: bigint; doubleValue: null }
    | { valueType: 'Double'; intValue: null; doubleValue: number }
    | { valueType: 'Empty'; intValue: null; doubleValue: null }
  )

export type ReceivedHistogramDataPoint = ReceivedDataPointBase & {
  count: bigint
  sum?: number
  min?: number
  max?: number
  bucketCounts: bigint[]
  explicitBounds: number[]
}

export type ReceivedExponentialHistogramDataPoint = ReceivedDataPointBase & {
  count: bigint
  sum?: number
  min?: number
  max?: number
  scale: number
  zeroCount: bigint
  zeroThreshold: number
  positive: { offset: number; bucketCounts: bigint[] }
  negative: { offset: number; bucketCounts: bigint[] }
}

export type ExactMetricSeries = ExactMetricIdentity & {
  seriesRef: string
  attributes: Attributes
  datapoints: (
    | ReceivedNumberDataPoint
    | ReceivedHistogramDataPoint
    | ReceivedExponentialHistogramDataPoint
  )[]
}

// Sparkline point shape used by detail charts (not the drawer summary).
export type SparklinePoint = {
  timestamp: bigint
  value: number
}

// Metric summary for sidebar cards.
export type MetricSummary = {
  metricRef: string
  name: string
  description: string
  unit: string
  metricType: MetricType
  aggregationTemporality: string | null
  aggregationTemporalityCode: number | null
  isMonotonic: boolean | null
  serviceName: string
  // Distinct attribute sets (timeseries) seen in the queried window.
  seriesCount: number
  seriesCardinality: number
  // In-range datapoints for this Metric.
  dataPointCount: number
  /** @derived Most recent Gauge/Sum value by timestamp in the requested
   * window. SQL coalesces double/int sources into an IEEE-754 metric-unit
   * number, so integer measurements past 2^53 are approximate. Null for
   * histograms. */
  lastValue: number | null
  // Timestamp of the most recent in-range datapoint (nanoseconds).
  lastSeen: bigint
}

export function metricSummaryKey(s: MetricSummary): string {
  return s.metricRef
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

/** Telemetry the store refused, one entry per signal and kind. */
export type Rejection = {
  signal: 'traces' | 'logs' | 'metrics'
  kind: string
  occurrences: number
  /** Most recently refused spans, newest first, wire form, bounded. Empty
   * when nothing in the store represents the refused rows. */
  samples: { traceID: string; spanID: string }[]
  firstSeen: bigint | null
  lastSeen: bigint | null
}

export type Stats = {
  traces: TraceStats
  logs: LogStats
  metrics: MetricStats
  rejections: Rejection[]
}

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
