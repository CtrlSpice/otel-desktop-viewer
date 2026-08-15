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

// The searchSpans wire shape is compressed in three ways, all resolved at the
// service boundary so nothing downstream sees them:
//
//   - resource and scope are references (`r`, `s`) into the response's
//     top-level maps rather than a full copy per span. The reference trace
//     holds 23 distinct resources and 1 scope across 5,735 spans, where the
//     repeated objects were over half the payload.
//   - times are `start` (offset from the response's traceStart) and `dur`,
//     rather than two 19-digit absolute nanosecond strings. Offset and
//     duration are also what a waterfall bar is: a position and a width.
//   - traceID is gone; it is at the response root, and a single-trace response
//     repeated it once per span.
//
// Together these took the reference trace from 6.90 MB to 2.92 MB.
export type JsonSpanData = {
  traceState: string
  spanID: string
  parentSpanID: string | null
  name: string
  kind: string
  /** Nanoseconds after JsonTraceData.traceStart. */
  start: number
  /** Duration in nanoseconds, measured from this span's own start. */
  dur: number
  // attributes/events/links are coalesced to [] server-side; never absent.
  attributes: JsonAttribute[]
  events: JsonEventData[]
  links: JsonLinkData[]
  /** Key into JsonTraceData.resources. */
  r: number
  /** Key into JsonTraceData.scopes. */
  s: number
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
  /**
   * Absolute nanoseconds, as a string: the baseline every span's `start` is
   * measured from. Only this one field needs the full magnitude.
   *
   * It is min(start_time) across the spans in *this response*, not the root
   * span's start -- clock skew across hosts means a child can legitimately
   * report an earlier start than its parent, and a trace may have no root at
   * all. Baseline and offsets are computed by the same query and shipped
   * together, so each response is internally consistent.
   */
  traceStart: string
  /** Distinct resources in this trace, keyed by a store-stable sequence number. */
  resources: Record<string, JsonResourceData>
  /** Distinct scopes in this trace, keyed by a store-stable sequence number. */
  scopes: Record<string, JsonScopeData>
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
  /** Quantile values keyed by the quantile, e.g. {"0.5": 12.4}. Computed in
   *  the store from this datapoint's buckets; null when none were requested.
   *  Keys are the quantile as the server formatted it, so look up by the same
   *  string the request sent. */
  quantiles: Record<string, number | null> | null
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
  /** Quantile values keyed by the quantile, e.g. {"0.5": 12.4}. Computed in
   *  the store from this datapoint's buckets; null when none were requested.
   *  Keys are the quantile as the server formatted it, so look up by the same
   *  string the request sent. */
  quantiles: Record<string, number | null> | null
  aggregationTemporality: string
}

export type JsonDataPoint =
  | JsonGaugeDataPoint
  | JsonSumDataPoint
  | JsonHistogramDataPoint
  | JsonExponentialHistogramDataPoint

export type JsonMetricTimeseries = {
  /**
   * The series id: content-derived from (stream, resource, labels).
   *
   * Was the canonical "key=value|..." rendering of the labels, which could not
   * survive series splitting by resource -- two replicas of one service have
   * byte-identical labels and so produced colliding keys. It is also stable
   * across restarts and retention, which the old key was not, so it can be put
   * in a URL.
   */
  attributesKey: string
  attributes: JsonAttribute[]
  /**
   * The resource that emitted this series.
   *
   * Load-bearing once series split by resource: when two replicas produce
   * identical labels, this is the only thing that tells them apart. Constant
   * within a series by construction. JsonMetricData.resource still describes
   * one arbitrary batch and is the weaker claim.
   */
  resource: JsonResourceData
  datapoints: JsonDataPoint[]
  /** Server-computed stats over the whole window; null for histograms. */
  stats: JsonSeriesValueStats | null
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
  /** The selected series merged into one histogram per time bucket -- what a
   *  heatmap draws and what a window summary describes. Null when the metric is
   *  not a histogram, or when no merge happened, so a client can tell that
   *  apart from "merged to nothing". */
  aggregate: JsonAggregateBucket[] | null
  /** Datapoints in the window, which may exceed the number returned. */
  datapointCount: number
}

/** One time bucket of the cross-series merge. Carries bucket vectors because
 *  the heatmap draws them; per-series data carries only quantiles. */
export type JsonAggregateBucket = {
  timestamp: string
  startTime: string
  count: number
  sum: number
  /** Derived from the buckets: a merge cannot carry the observed min and max
   *  through, because for cumulative it is a subtraction. */
  min: number
  max: number
  /** Explicit-bounds histograms carry these; exponential ones carry the
   *  scale/offset fields below. A bucket has one representation or the other,
   *  never both, so the absent set is omitted rather than sent as nulls. */
  bucketCounts?: number[]
  explicitBounds?: number[]
  scale?: number
  zeroThreshold?: number
  zeroCount?: number
  positiveBucketOffset?: number
  positiveBucketCounts?: number[]
  negativeBucketOffset?: number
  negativeBucketCounts?: number[]
  quantiles: Record<string, number | null> | null
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

// searchAttributes: value-first discovery.
//
// The getXAttributes methods answer "which keys exist" so a dropdown can be
// filled. This answers the opposite question -- "I can see this text, which key
// is it?" -- and does it across traces, logs and metrics in one call, because
// they all reference the same attribute dictionary.
//
// matchCount is the number of distinct *values* of this key that match, not the
// number of spans or logs carrying it. It distinguishes "this term identifies
// one specific thing" from "this term appears all over a high-cardinality key".
// sampleValues is a short, bounded illustration, not a complete list.
export type JsonAttributeMatch = JsonAttributeDefinition & {
  matchCount: number
  sampleValues: string[]
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

/** Per-series value statistics computed by the store over every datapoint in
 *  the window, as opposed to the subset a chart draws. */
export type JsonSeriesValueStats = {
  count: number
  min: number
  max: number
  sum: number
  avg: number
}
