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

// Discriminated union for search results
export type SearchResultEvent =
  | { signal: 'traces'; view: 'list'; results: TraceSummary[] }
  | { signal: 'traces'; view: 'detail'; results: TraceData }
  | { signal: 'logs'; view: 'list'; results: LogData[] }
  | { signal: 'metrics'; view: 'list'; results: MetricData[] }
  | { signal: 'metrics'; view: 'detail'; results: MetricData[] }
