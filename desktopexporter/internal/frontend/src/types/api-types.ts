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

type BaseDataPoint = {
  id: string
  timestamp: bigint
  startTime: bigint
  attributes: Attributes
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
  datapoints: DataPoint[]
}

// Metric summary for sidebar cards (lightweight, grouped by OTel identity)
export type SparklinePoint = {
  timestamp: bigint
  value: number
}

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
  sparkline: SparklinePoint[] | null
  sparkbar: number[] | null
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

// Quantile series (trend chart) types. The backend computes adaptive time
// buckets and emits one point per (bucket, stream) for per-stream mode and
// one point per bucket for aggregated mode.
export type QuantileSeriesMode = 'per-stream' | 'aggregated'

// One point in a quantile series. `quantiles` keys are the same float
// strings produced by Go's strconv.FormatFloat with -1 precision (e.g.
// "0.5", "0.95"); a value of null means the macro declined to interpolate
// (empty buckets / total count of zero) and should render as a dash.
// `attributes` and `attributesKey` are empty/blank for aggregated mode.
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
export type BucketSeriesMode = 'per-stream' | 'aggregated'

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
