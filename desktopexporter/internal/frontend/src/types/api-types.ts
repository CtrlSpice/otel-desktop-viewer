export type RootSpan = {
  serviceName: string
  name: string
  startTime: bigint
  endTime: bigint
}

export type TraceSummary = {
  traceID: string
  rootSpan?: RootSpan
  spanCount: number
  errorCount: number
  exceptionCount: number
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

// Metrics types
export type MetricType =
  | 'Empty'
  | 'Gauge'
  | 'Sum'
  | 'Histogram'
  | 'ExponentialHistogram'

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
export type MetricTimeseries = {
  attributesKey: string
  attributes: Attributes
  datapoints: DataPoint[]
}

export type MetricData = {
  id: string
  name: string
  description: string
  unit: string
  resourceDroppedAttributesCount: number
  resource: ResourceData
  scopeName: string
  scopeVersion: string
  scopeDroppedAttributesCount: number
  scope: ScopeData
  received: bigint
  timeseries: MetricTimeseries[]
}

// Metric summary for sidebar cards (lightweight, grouped by OTel identity)
export type SparklinePoint = {
  timestamp: bigint
  value: number
}

// Reasons a spark may be omitted in lieu of data. The string discriminant lets
// the frontend pick the right callout (e.g. the unspecifiedTemporality meme)
// without coupling to the JSON-RPC error catalogue.
export type SparkErrorReason = 'unspecifiedTemporality'

// Discriminated union for spark payloads. Mirrors how trace summaries handle
// a missing root span: the data shape itself encodes the situation, so callers
// just pattern-match. Three states because we need to distinguish "we refused
// to compute" from "no data in range".
//   - { kind: 'data', value }: render as a chart.
//   - { kind: 'error', reason }: render the FunError leaf for that reason.
//   - null: legitimate empty (no data in range).
export type SparkOutcome<T> =
  | { kind: 'data'; value: T }
  | { kind: 'error'; reason: SparkErrorReason }
  | null

export type Sparkline = SparkOutcome<SparklinePoint[]>
export type Sparkbar = SparkOutcome<number[]>

export type MetricSummary = {
  name: string
  description: string
  unit: string
  metricType: MetricType
  aggregationTemporality: string | null
  isMonotonic: boolean | null
  serviceName: string
  scopeName: string
  scopeVersion: string
  received: bigint
  sparkline: Sparkline
  sparkbar: Sparkbar
}

export function metricSummaryKey(s: MetricSummary): string {
  return `${s.name}::${s.unit}::${s.metricType}::${s.aggregationTemporality ?? ''}::${s.isMonotonic ?? ''}::${s.scopeName}::${s.scopeVersion}::${s.serviceName}`
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
export type SearchResultEvent =
  | { signal: 'traces'; results: TraceSummary[]; queryTree?: unknown }
  | { signal: 'logs'; results: LogData[]; queryTree?: unknown }
  | { signal: 'metrics'; results: MetricData[]; queryTree?: unknown }

// Quantile series (trend chart) types. The backend computes adaptive
// time buckets and emits one point per (bucket, timeseries) for
// per-attribute mode and one point per bucket for merged mode.
//
// Modes mirror the OTel-aligned terminology used elsewhere: a
// "timeseries" is one per-attribute stream within a metric, so
// 'per-attribute' returns one point per (bucket, attribute set), and
// 'merged' folds all timeseries into a single point per bucket.
export type QuantileSeriesMode = 'per-attribute' | 'merged'

// One point in a quantile series. `quantiles` keys are the same float
// strings produced by Go's strconv.FormatFloat with -1 precision (e.g.
// "0.5", "0.95"); a value of null means the macro declined to interpolate
// (empty buckets / total count of zero) and should render as a dash.
// `attributes` and `attributesKey` are empty/blank for merged mode.
export type QuantileSeriesPoint = {
  timestamp: bigint
  attributesKey: string
  attributes: Attributes
  quantiles: Record<string, number | null>
  count: number
  sum: number
  min: number | null
  max: number | null
}

// Bucket series (heatmap) types. Same adaptive bucketing as quantile series,
// but the raw bucket vectors are returned instead of computed quantiles.
export type BucketSeriesMode = 'per-attribute' | 'merged'

export type BucketSeriesTotals = {
  count: number
  sum: number
  min: number | null
  max: number | null
}

export type HistogramBucketPoint = {
  kind: 'histogram'
  timestamp: bigint
  attributesKey: string
  attributes: Attributes
  bounds: number[]
  counts: number[]
  totals: BucketSeriesTotals
}

export type ExpHistogramBucketPoint = {
  kind: 'expHistogram'
  timestamp: bigint
  attributesKey: string
  attributes: Attributes
  scale: number
  zeroThreshold: number
  zeroCount: number
  positiveOffset: number
  positiveCounts: number[]
  negativeOffset: number
  negativeCounts: number[]
  totals: BucketSeriesTotals
}

export type BucketSeriesPoint = HistogramBucketPoint | ExpHistogramBucketPoint
