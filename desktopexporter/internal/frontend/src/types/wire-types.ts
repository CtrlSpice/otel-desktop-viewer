// Wire types: the JSON shapes the backend actually serves, verified against
// the DuckDB json_object projections in internal/store/{spans,logs,metrics,
// stats} and the JSON-RPC handler. These deliberately duplicate the domain
// types in api-types.ts rather than deriving from them -- the wire contract
// should only change when the backend changes, not when a frontend type is
// edited.
//
// The one systematic wire/domain difference: int64 nanosecond timestamps
// ride as strings (JSON numbers are float64 and would clip ns precision)
// and are promoted to bigint by the revivers in telemetry-service.ts.
//
// Timestamps only: other 64-bit fields (datapoint intValue, histogram
// count/zeroCount/bucket arrays, summary lastValue) ride as JSON numbers
// and silently lose precision past 2^53. That is the current contract,
// not an oversight in these types.

export type JsonAttribute = {
  key: string
  value: string
  type: string
}

export type JsonResourceData = {
  attributes: JsonAttribute[]
  droppedAttributesCount: number
}

export type JsonScopeData = {
  name: string
  version: string
  attributes: JsonAttribute[]
  droppedAttributesCount: number
}

// --- Traces ---

export type JsonRootSpan = {
  // nullif(service_name, '') in the summary projection: an empty service
  // name arrives as JSON null, not ''.
  serviceName: string | null
  name: string
}

export type JsonTraceSummary = {
  traceID: string
  hasRootSpan: boolean
  // SQL `case ... end` without else: orphaned traces get null (not absent).
  rootSpan: JsonRootSpan | null
  startTime: string
  durationNs: string | null
  spanCount: number
  errorCount: number
}

export type JsonEventData = {
  name: string
  timestamp: string
  droppedAttributesCount: number
  attributes: JsonAttribute[]
}

export type JsonLinkData = {
  traceID: string
  // The linked target span (OTLP link.spanID), 16-char hex.
  spanID: string
  traceState: string
  droppedAttributesCount: number
  attributes: JsonAttribute[]
}

export type JsonSpanData = {
  traceID: string
  traceState: string
  spanID: string
  parentSpanID: string | null
  name: string
  kind: string
  startTime: string
  endTime: string
  // attributes/events/links are coalesced to [] server-side; never absent.
  attributes: JsonAttribute[]
  events: JsonEventData[]
  links: JsonLinkData[]
  resource: JsonResourceData
  scope: JsonScopeData
  droppedAttributesCount: number
  droppedEventsCount: number
  droppedLinksCount: number
  statusCode: string
  statusMessage: string
}

export type JsonSpanNode = {
  spanData: JsonSpanData
  depth: number
  // Always emitted: literal true when no search criteria, else per-span.
  matched: boolean
}

export type JsonTraceData = {
  traceID: string
  spans: JsonSpanNode[]
}

// --- Logs ---

export type JsonLogSummary = {
  id: string
  timestamp: string
  severityText: string
  severityNumber: number
  serviceName: string
  bodyPreview: string
}

export type JsonLogData = {
  id: string
  timestamp: string
  observedTimestamp: string
  traceID: string | null
  spanID: string | null
  severityText: string
  severityNumber: number
  body: string
  bodyType: string
  resource: JsonResourceData
  scope: JsonScopeData
  droppedAttributesCount: number
  flags: number
  eventName: string
  attributes: JsonAttribute[]
}

// --- Metrics ---

// Same closed set as MetricType in api-types.ts, duplicated deliberately
// so the wire contract stays fully self-contained.
export type JsonMetricType =
  'Empty' | 'Gauge' | 'Sum' | 'Histogram' | 'ExponentialHistogram'

export type JsonExemplar = {
  timestamp: string
  value: number
  traceID: string | null
  spanID: string | null
  filteredAttributes: JsonAttribute[]
}

// Datapoints are json_merge_patch(base, per-type object); the per-type
// field sets mirror the DataPoint union in api-types.ts exactly (they carry
// no int64-as-string fields), so only the base timestamps differ.
type JsonBaseDataPoint = {
  id: string
  timestamp: string
  startTime: string
  flags: number
  exemplars: JsonExemplar[]
}

export type JsonGaugeDataPoint = JsonBaseDataPoint & {
  metricType: 'Gauge'
  doubleValue: number | null
  intValue: number | null
  valueType: string
}

export type JsonSumDataPoint = JsonBaseDataPoint & {
  metricType: 'Sum'
  doubleValue: number | null
  intValue: number | null
  valueType: string
  isMonotonic: boolean
  aggregationTemporality: string
}

export type JsonHistogramDataPoint = JsonBaseDataPoint & {
  metricType: 'Histogram'
  count: number
  sum: number
  min: number
  max: number
  bucketCounts: number[]
  explicitBounds: number[]
  aggregationTemporality: string
}

export type JsonExponentialHistogramDataPoint = JsonBaseDataPoint & {
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

export type JsonDataPoint =
  | JsonGaugeDataPoint
  | JsonSumDataPoint
  | JsonHistogramDataPoint
  | JsonExponentialHistogramDataPoint

export type JsonMetricTimeseries = {
  attributesKey: string
  attributes: JsonAttribute[]
  datapoints: JsonDataPoint[]
}

export type JsonMetricData = {
  id: string
  name: string
  // Coalesced server-side ('' for a stream with no ingests in the window).
  description: string
  unit: string
  metricType: JsonMetricType
  // Projected straight off metric_streams, where "not applicable" is
  // encoded as '' / false (not-null columns; the encoding keeps the
  // stream UNIQUE constraint deduping correctly). Contrast with
  // JsonMetricSummary, which nulls isMonotonic for non-Sum types.
  aggregationTemporality: string
  isMonotonic: boolean
  resourceDroppedAttributesCount: number
  resource: JsonResourceData
  scopeName: string
  scopeVersion: string
  scopeDroppedAttributesCount: number
  scope: JsonScopeData
  timeseries: JsonMetricTimeseries[]
}

export type JsonMetricSummary = {
  id: string
  name: string
  // Left-joined from stream_description; null when absent.
  description: string | null
  unit: string
  metricType: JsonMetricType
  // Not-null stream column; '' encodes "not applicable" (see
  // JsonMetricData).
  aggregationTemporality: string
  // Explicitly nulled by the projection for every type except Sum --
  // unlike getMetric, which serves the raw stream column (false for
  // non-Sums).
  isMonotonic: boolean | null
  serviceName: string
  // seriesCount/dataPointCount/lastSeen all come from left joins, but the
  // search time-condition guarantees every filtered stream has at least
  // one in-window datapoint, so all three CTEs always produce a row.
  seriesCount: number
  dataPointCount: number
  lastValue: number | null
  lastSeen: string
}

// --- Stats ---

export type JsonTraceStats = {
  traceCount: number
  spanCount: number
  serviceCount: number
  errorCount: number
  // max() over an empty table -> null.
  lastReceived: string | null
}

export type JsonLogStats = {
  logCount: number
  errorCount: number
  lastReceived: string | null
}

export type JsonMetricStats = {
  metricCount: number
  dataPointCount: number
  lastReceived: string | null
}

export type JsonStats = {
  // Served by the backend but not yet consumed by the UI.
  storage: {
    sizeBytes: number
    maxSizeBytes: number
  }
  traces: JsonTraceStats
  logs: JsonLogStats
  metrics: JsonMetricStats
}

// --- Attribute discovery (getTraceAttributes / getLogAttributes /
// getMetricAttributes / getAttributesByTraceID) ---

// The attr_type DuckDB enum verbatim (schema.go). Note the spelling
// asymmetry the enum itself carries: scalar booleans are 'bool', arrays
// are 'boolean[]'. The frontend's FieldType uses 'boolean' for both, so
// the service layer translates the scalar at the boundary.
export type JsonAttributeType =
  | 'string'
  | 'int64'
  | 'float64'
  | 'bool'
  | 'string[]'
  | 'int64[]'
  | 'float64[]'
  | 'boolean[]'

// Union across the discovery endpoints: traces serve resource/scope/span/
// event/link, logs serve resource/scope/log, metrics serve resource/scope/
// datapoint/exemplar.
//
// datapoint and exemplar arrived with the attribute dictionary. Before it,
// metric discovery deliberately stopped at the per-batch resource and scope
// rows: reaching datapoint labels meant a second join through a table where
// they were 82% of the rows, on the interactive path that fills the search
// dropdowns. Reading them from the dictionary is the same `select distinct`,
// so they are now both discoverable and searchable.
//
// Note the scope is the *owner kind*, not the storage location -- an attribute
// id implies its scope because scope is part of the content hash.
export type JsonAttributeScope =
  | 'resource'
  | 'scope'
  | 'span'
  | 'event'
  | 'link'
  | 'log'
  | 'datapoint'
  | 'exemplar'

export type JsonAttributeDefinition = {
  name: string
  attributeScope: JsonAttributeScope
  type: JsonAttributeType
}

// --- Mutation results ---

// deleteSpansByTraceID / deleteSpanByID / deleteLogByID.
// `count` is the number of IDs accepted, not rows removed.
export type JsonDeleteResult = {
  message: string
  count: number
}

// --- Request direction (frontend -> backend): the query-tree shape
// search.ParseQueryTree unmarshals on the Go side
// (internal/store/search/search_tree.go) ---

export type JsonQueryField = {
  // name/type are omitted for global search; attributeScope only
  // accompanies attribute-scoped fields.
  name?: string
  type?: string
  searchScope: string
  attributeScope?: string
}

export type JsonQueryNode =
  | {
      id: string
      type: 'condition'
      query: {
        field: JsonQueryField
        fieldOperator: string
        value: string
      }
    }
  | {
      id: string
      type: 'group'
      group: {
        logicalOperator: string
        children: JsonQueryNode[]
      }
    }
