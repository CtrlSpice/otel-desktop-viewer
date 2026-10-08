// JSON shapes served by the backend. These remain independent of domain types.
//
// Received 64-bit integers ride as decimal strings because JSON numbers would
// clip them past 2^53. The service revivers promote timestamps, integer metric
// measurements and histogram counts to bigint. Derived scalar summaries,
// rates, quantiles and chart coordinates remain numbers at their documented
// approximation boundaries.

export type JsonAttribute = {
  id?: string
  key: string
  value: JsonAttributeValue
}

export type JsonAttributeValue =
  | { kind: 'empty'; value: null }
  | { kind: 'string' | 'bytes'; value: string }
  | { kind: 'bool'; value: boolean }
  | { kind: 'int64'; value: string }
  | { kind: 'double'; value: number | string }
  | { kind: 'array'; value: JsonAttributeValue[] }
  | { kind: 'map'; value: JsonAttributeMapEntry[] }

export type JsonAttributeMapEntry = {
  key: string
  value: JsonAttributeValue
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

export type JsonRootSpan = {
  // nullif(service_name, '') in the summary projection: an empty service
  // name arrives as JSON null, not ''.
  serviceName: string | null
  name: string
}

export type JsonMatchedSpan = {
  traceID: string
  spanID: string
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
  /** Computed identities of matching spans; present only for filtered searches. */
  matchedSpans?: JsonMatchedSpan[]
}

export type JsonEventData = {
  name: string
  timestamp: string
  droppedAttributesCount: number
  attributes: JsonAttribute[]
}

export type JsonLinkData = {
  traceID: string | null
  // The linked target span (OTLP link.spanID), 16-char hex when valid.
  spanID: string | null
  traceState: string
  droppedAttributesCount: number
  /** W3C trace flags for the linked context. Same reasoning as on the span. */
  flags: number
  attributes: JsonAttribute[]
}

// Spans reference top-level resources and scopes, use baseline-relative times,
// and inherit traceID from the response root. The service resolves this shape.
export type JsonSpanData = {
  traceState: string
  spanID: string
  parentSpanID: string | null
  /** Received W3C trace flags, including the remote-parent bit. */
  flags: number
  name: string
  /** Authoritative received OTLP SpanKind int32. */
  kindCode: number
  /** Readable label derived from kindCode by SQL. */
  kind: string
  /** Nanoseconds after JsonTraceData.traceStart, kept exact across JSON. */
  start: string
  /** Duration in nanoseconds, measured from this span's own start. */
  dur: string
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
  /** Authoritative received OTLP StatusCode int32. */
  statusCodeValue: number
  /** Readable label derived from statusCodeValue by SQL. */
  statusCode: string
  statusMessage: string
}

export type JsonSpanNode = {
  spanData: JsonSpanData
  depth: number
  // Always emitted: literal true when no search criteria, else per-span.
  matched: boolean
  /**
   * Present only on spans recovered from a stranded part of the trace -- ones
   * the ordinary walk could not reach because their parent links form a loop.
   * Absent on every span of a well-formed trace, which is why it is optional
   * rather than a boolean on every row.
   */
  salvaged?: true
  /**
   * Present alongside `salvaged`. True on the retained root of a salvaged
   * chain when its reported parent appears further down that same chain.
   * This identifies the display cut, not which parent assignment is wrong.
   */
  cyclePoint?: boolean
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
  /**
   * Spans present in the trace that could not be placed under any root, and
   * so are absent from `spans`.
   *
   * Normally 0. A span whose parent is missing is promoted to a root and
   * rendered as its own tree, so the only way to be unplaced is to sit on a
   * parent cycle -- malformed input. Reported rather than silently dropped,
   * because otherwise the trace just renders short.
   */
  unplacedSpanCount: number
  spans: JsonSpanNode[]
}

export type JsonLogSummary = {
  logRef: string
  timestamp: string
  severityText: string
  severityNumber: number
  serviceName: string
  bodyPreview: string
}

export type JsonTraceLogSummary = JsonLogSummary & {
  spanID: string | null
  eventName: string
}

export type JsonLogData = {
  logRef: string
  timestamp: string
  observedTimestamp: string
  traceID: string | null
  spanID: string | null
  severityText: string
  severityNumber: number
  body: JsonAttributeValue
  resource: JsonResourceData
  scope: JsonScopeData
  droppedAttributesCount: number
  flags: number
  eventName: string
  attributes: JsonAttribute[]
}

// Same closed set as MetricType, kept independent for the wire contract.
export type JsonMetricType =
  'Empty' | 'Gauge' | 'Sum' | 'Histogram' | 'ExponentialHistogram'

/** A JSON number, or exact IEEE-754 bits when JSON cannot represent the value. */
export type JsonDouble = number | `0x${string}`

type JsonExemplarBase = {
  timestamp: string
  traceID: string | null
  spanID: string | null
  filteredAttributes: JsonAttribute[]
}

export type JsonExemplar = JsonExemplarBase &
  (
    | { valueType: 'Double'; doubleValue: JsonDouble; intValue: null }
    | { valueType: 'Int'; doubleValue: null; intValue: string }
    | { valueType: 'Empty'; doubleValue: null; intValue: null }
  )

// Datapoints are json_merge_patch(base, per-type object); the per-type
// field sets mirror the DataPoint union in api-types.ts, with received 64-bit
// integers encoded as decimal strings and revived at the service boundary.
type JsonBaseDataPoint = {
  id: string
  timestamp: string
  /** The same instant in epoch milliseconds, as a number. Charts want ms and
   *  would otherwise divide the nanosecond BigInt once per datapoint. */
  timestampMs: number
  startTime: string
  flags: number
  exemplars: JsonExemplar[]
  /** How many exemplars this datapoint holds, sent only when that exceeds how
   *  many arrived -- the store caps the list so one aggressively sampled stream
   *  cannot decide the size of the response. Absent is the ordinary case and
   *  means nothing was withheld, so read it as `exemplars.length`. */
  exemplarCount?: number
}

export type JsonGaugeDataPoint = JsonBaseDataPoint & {
  metricType: 'Gauge'
  doubleValue: JsonDouble | null
  intValue: string | null
  valueType: string
}

export type JsonSumDataPoint = JsonBaseDataPoint & {
  metricType: 'Sum'
  doubleValue: JsonDouble | null
  intValue: string | null
  valueType: string
  isMonotonic: boolean
  /** Authoritative received OTLP AggregationTemporality int32. */
  aggregationTemporalityCode: number
  /** Readable label derived from aggregationTemporalityCode by SQL. */
  aggregationTemporality: string
  /** Activity since the previous reading of this series. Exact integral
   *  results use decimal text; double-domain results use JsonDouble.
   *  Cumulative only; null on the first datapoint. */
  delta?: JsonDouble | string | null
  /** Whether the counter restarted in that interval. */
  isReset?: boolean | null
}

export type JsonHistogramDataPoint = JsonBaseDataPoint & {
  metricType: 'Histogram'
  count: string
  sum: JsonDouble | null
  min: JsonDouble | null
  max: JsonDouble | null
  bucketCounts: string[]
  explicitBounds: JsonDouble[]
  /** Quantile values keyed by the quantile, e.g. {"0.5": 12.4}. Computed in
   *  the store from this datapoint's buckets; null when none were requested.
   *  Keys are the quantile as the server formatted it, so look up by the same
   *  string the request sent. */
  quantiles: Record<string, JsonDouble | null> | null
  aggregationTemporalityCode: number
  aggregationTemporality: string
}

export type JsonExponentialHistogramDataPoint = JsonBaseDataPoint & {
  metricType: 'ExponentialHistogram'
  count: string
  sum: JsonDouble | null
  min: JsonDouble | null
  max: JsonDouble | null
  scale: number
  zeroCount: string
  zeroThreshold: JsonDouble
  positiveBucketOffset: number
  positiveBucketCounts: string[]
  negativeBucketOffset: number
  negativeBucketCounts: string[]
  /** Quantile values keyed by the quantile, e.g. {"0.5": 12.4}. Computed in
   *  the store from this datapoint's buckets; null when none were requested.
   *  Keys are the quantile as the server formatted it, so look up by the same
   *  string the request sent. */
  quantiles: Record<string, JsonDouble | null> | null
  aggregationTemporalityCode: number
  aggregationTemporality: string
}

export type JsonDataPoint =
  | JsonGaugeDataPoint
  | JsonSumDataPoint
  | JsonHistogramDataPoint
  | JsonExponentialHistogramDataPoint

/** One bucket of a scalar series' Sum / Average / Rate views, as the store
 *  computed them. sum, avg and rate are null when the bucket holds no samples:
 *  Sum and Rate read that as no activity, Average has to skip it, and a zero
 *  here would decide that for both. */
export type JsonScalarViewBucket = {
  bucketStart: string
  sampleCount: number
  sum: JsonDouble | null
  avg: JsonDouble | null
  rate: JsonDouble | null
  /** Slope of the drawn rate line's segment arriving at this bucket, in
   *  rate-units per second. Null for buckets the rate view does not draw and
   *  for the first it does, which no segment arrives at. */
  slope: JsonDouble | null
  hasReset: boolean
}

/** Extremes of a series' drawn rate line, for the rate view's badges. */
export type JsonSeriesRateStats = {
  min: JsonDouble
  max: JsonDouble
  avg: JsonDouble
}

export type JsonMetricSeriesViewData = {
  /** Generated database-local series reference. */
  seriesRef: string
  attributes: JsonAttribute[]
  /** Resource associated with the parent Metric. */
  resource: JsonResourceData
  datapoints: JsonDataPoint[]
  /** Server-computed stats over the whole window; null for histograms. */
  stats: JsonSeriesValueStats | null
  /** Datapoints the window holds for this series, before narrowing and before
   *  the reduction — so not the length of `datapoints`, which is what this
   *  response happens to carry. */
  datapointCount: number
  /** When this series last reported in the window, ns as a string. */
  lastSeenNs: string | null
  /** Extremes of the drawn rate line; null for histograms and for series with
   *  no rate to draw. The raw stats describe the values, these the transform. */
  rateStats: JsonSeriesRateStats | null
  /** Per-bucket Sum / Average / Rate. Null for histogram series, which have no
   *  scalar to aggregate. */
  views: JsonScalarViewBucket[] | null
  /** This series' shape at list-row resolution: the store's min and max per
   *  bucket, sized for the 128px sparkline box rather than the chart. Sent for
   *  every series, including unchecked ones -- the sparkline is how a user
   *  decides what to check. Null for histograms, and when the caller asked for
   *  no sparkline buckets. */
  sparkline: JsonSparklinePoint[] | null
}

/** One point of a series' row sparkline. A bucket contributes its min and its
 *  max, each at the timestamp it actually occurred, so a spike leans the way it
 *  happened rather than being squared off to a bucket boundary. */
export type JsonSparklinePoint = {
  timestamp: string
  value: JsonDouble
}

export type JsonMetricViewData = {
  /** The window's most recent datapoint across every series, ns as a string.
   *  Independent of which series shipped datapoints. */
  lastSeenNs: string | null
  metricRef: string
  name: string
  // Coalesced server-side ('' for a stream with no ingests in the window).
  description: string
  /**
   * OTLP Metric.metadata: an attribute map describing the instrument itself,
   * not the labels that identify a series. This is the latest received value.
   * Coalesced to [] server-side.
   */
  metadata: JsonAttribute[]
  unit: string
  metricType: JsonMetricType
  // Numeric code is authoritative. Gauge's stored zero is non-applicable by
  // metricType, so its derived label is null rather than "Unspecified".
  aggregationTemporalityCode: number | null
  aggregationTemporality: string | null
  isMonotonic: boolean | null
  resourceDroppedAttributesCount: number
  /** Latest received ResourceMetrics schema URL for this Metric. */
  resourceSchemaUrl: string
  resource: JsonResourceData
  scopeName: string
  scopeVersion: string
  scopeSchemaUrl: string
  scopeDroppedAttributesCount: number
  scope: JsonScopeData
  timeseries: JsonMetricSeriesViewData[]
  /** The selected series merged into one histogram per time bucket -- what a
   *  heatmap draws and what a window summary describes. Null when the metric is
   *  not a histogram, or when no merge happened, so a client can tell that
   *  apart from "merged to nothing". */
  aggregate: JsonAggregateBucket[] | null
  /** The cross-series lines for a scalar metric, in the same bucket shape the
   *  per-series views use. `selected` is empty when nothing is checked, which
   *  the chart reads as "draw All by itself". Both are empty for histograms. */
  scalarAggregate: JsonScalarAggregate | null
  /** Datapoints in the window, which may exceed the number returned. */
  datapointCount: number
  /** Merges the store refused because their inputs disagreed about explicit
   *  bounds, or null when none were. Bounds cannot be reconciled the way scales
   *  can, so the rows are dropped -- and a dropped bucket is indistinguishable
   *  from one that never had data, which is why this is reported rather than
   *  left to be noticed. */
  boundsMismatch: JsonBoundsMismatch | null
  /** Requested bounds and the concrete bounds the reduction used. A missing
   * requested endpoint is independently filled from the filtered data extent;
   * it remains null in `effective` when the result has no such extent. */
  window: {
    requested: { startNs: string | null; endNs: string | null }
    effective: { startNs: string | null; endNs: string | null }
  }
}

/** One time bucket of the cross-series merge. Carries bucket vectors because
 *  the heatmap draws them; per-series data carries only quantiles. */
/** Both cross-series pools. `all` never narrows with the selection -- that is
 *  what makes it "all" -- while `selected` follows the checkboxes. */
export type JsonBoundsMismatch = {
  /** (series, bucket) merges refused along the time axis. */
  seriesBuckets: number
  /** Cross-series bucket merges refused, by either route: a contributing
   *  series that could not merge within itself, or series that disagree with
   *  one another. */
  aggregateBuckets: number
}

export type JsonScalarAggregate = {
  selected: JsonScalarViewBucket[]
  all: JsonScalarViewBucket[]
}

/** What getMetricAggregateView returns: one envelope serving both metric shapes,
 *  each field null or empty on the shape it does not apply to. */
export type JsonMetricAggregateViewData = {
  aggregate: JsonAggregateBucket[] | null
  scalarAggregate: JsonScalarAggregate | null
}

export type JsonExactMetricResource = {
  attributes: JsonAttribute[]
  droppedAttributesCount: number
  schemaUrl: string
}

export type JsonExactMetricScope = {
  name: string
  version: string
  attributes: JsonAttribute[]
  droppedAttributesCount: number
  schemaUrl: string
}

type JsonExactMetricIdentityBase = {
  metricRef: string
  name: string
  description: string
  unit: string
  metadata: JsonAttribute[]
  resource: JsonExactMetricResource
  scope: JsonExactMetricScope
}

export type JsonExactMetricIdentity = JsonExactMetricIdentityBase &
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

export type JsonMetricSeriesSummary = {
  seriesRef: string
  attributes: JsonAttribute[]
  /** Computed count of retained datapoints in this series. */
  datapointCount: string
  /** Computed earliest received datapoint timestamp, or null with no points. */
  firstDatapointTimestamp: string | null
  /** Computed latest received datapoint timestamp, or null with no points. */
  lastDatapointTimestamp: string | null
}

export type JsonExactMetric = JsonExactMetricIdentity & {
  series: JsonMetricSeriesSummary[]
}

type JsonReceivedDataPointBase = {
  /** Viewer-generated database reference for this retained datapoint. */
  datapointRef: string
  timestamp: string
  startTime: string
  flags: number
  exemplars: JsonExemplar[]
}

export type JsonReceivedNumberDataPoint = JsonReceivedDataPointBase &
  (
    | { valueType: 'Int'; intValue: string; doubleValue: null }
    | { valueType: 'Double'; intValue: null; doubleValue: JsonDouble }
    | { valueType: 'Empty'; intValue: null; doubleValue: null }
  )

export type JsonReceivedHistogramDataPoint = JsonReceivedDataPointBase & {
  count: string
  sum?: JsonDouble
  min?: JsonDouble
  max?: JsonDouble
  bucketCounts: string[]
  explicitBounds: JsonDouble[]
}

export type JsonReceivedExponentialHistogramDataPoint =
  JsonReceivedDataPointBase & {
    count: string
    sum?: JsonDouble
    min?: JsonDouble
    max?: JsonDouble
    scale: number
    zeroCount: string
    zeroThreshold: JsonDouble
    positive: { offset: number; bucketCounts: string[] }
    negative: { offset: number; bucketCounts: string[] }
  }

export type JsonExactMetricSeries = JsonExactMetricIdentity & {
  seriesRef: string
  attributes: JsonAttribute[]
  datapoints: (
    | JsonReceivedNumberDataPoint
    | JsonReceivedHistogramDataPoint
    | JsonReceivedExponentialHistogramDataPoint
  )[]
}

/** @derived Cross-series histogram view computed by get_metric_view.sql. The store
 * differences cumulative inputs or adds delta inputs within each time bucket,
 * then adds aligned series vectors. Timestamps/start times are epoch ns decimal
 * text. Counts are JSON numbers for charting and can approximate integers past
 * 2^53; sum/min/max/quantiles are metric-unit IEEE-754 display values, with
 * min/max inferred from populated bucket extents and quantiles interpolated. */
export type JsonAggregateBucket = {
  timestamp: string
  startTime: string
  count: number
  sum: JsonDouble | null
  /** Derived from the buckets: a merge cannot carry the observed min and max
   *  through, because for cumulative it is a subtraction. Omitted when an
   *  empty explicit-bounds vector provides no finite extent. */
  min?: JsonDouble
  max?: JsonDouble
  /** Explicit-bounds histograms carry these; exponential ones carry the
   *  scale/offset fields below. A bucket has one representation or the other,
   *  never both, so the absent set is omitted rather than sent as nulls. */
  bucketCounts?: number[]
  explicitBounds?: JsonDouble[]
  scale?: number
  zeroThreshold?: JsonDouble
  zeroCount?: number
  positiveBucketOffset?: number
  positiveBucketCounts?: number[]
  negativeBucketOffset?: number
  negativeBucketCounts?: number[]
  quantiles: Record<string, JsonDouble | null> | null
}

export type JsonMetricSummary = {
  metricRef: string
  name: string
  // Left-joined from stream_description; null when absent.
  description: string | null
  unit: string
  metricType: JsonMetricType
  aggregationTemporalityCode: number | null
  aggregationTemporality: string | null
  // Explicitly nulled by the projection for every type except Sum --
  // unlike getMetricView, which serves the raw stream column (false for
  // non-Sums).
  isMonotonic: boolean | null
  serviceName: string
  // seriesCount/dataPointCount/lastSeen all come from left joins, but the
  // search time-condition guarantees every filtered stream has at least
  // one in-window datapoint, so all three CTEs always produce a row.
  /** Series that reported inside the requested window. */
  seriesCount: number
  /** Series the stream has ever had, whatever the window. Equal to
   *  seriesCount on an unbounded range, lower than it never. */
  seriesCardinality: number
  dataPointCount: number
  /** @derived Latest Gauge/Sum value by timestamp in the requested window.
   * SQL coalesces the double/int arms into an IEEE-754 metric-unit number, so
   * an integer source may be approximate past 2^53. Null for histograms. */
  lastValue: JsonDouble | null
  lastSeen: string
}

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

// Telemetry the store would not write, one row per signal and kind. Empty in
// the ordinary case. Ordered by recency by the backend.
export type JsonRejection = {
  signal: 'traces' | 'logs' | 'metrics'
  kind: string
  occurrences: number
  // The most recently refused spans, newest first, deduped, bounded at write
  // time. Wire form, minted by SQL: the struct fields are named for the wire.
  samples: { traceID: string; spanID: string }[]
  firstSeen: string
  lastSeen: string
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
  rejections: JsonRejection[]
}

// Actual received root OTel kinds. The frontend's FieldType uses 'boolean'
// for the wire spelling 'bool', translated at the service boundary.
export type JsonAttributeType =
  'string' | 'int64' | 'double' | 'bool' | 'bytes' | 'empty' | 'array' | 'map'

// Union across all discovery endpoints. Scope is the owner kind, not the storage
// location. Attribute ID implies scope because scope is part of the content hash.
export type JsonAttributeScope =
  | 'resource'
  | 'scope'
  | 'span'
  | 'event'
  | 'link'
  | 'log'
  | 'datapoint'
  | 'exemplar'
  | 'metadata'

export type JsonAttributeDefinition = {
  name: string
  attributeScope: JsonAttributeScope
  type: JsonAttributeType
}

// searchAttributeMatches: value-first discovery.
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
  sampleValues: JsonAttributeValue[]
}

// deleteSpansByTraceID / deleteLogsByRefs.
// `count` is the number of IDs accepted, not rows removed.
export type JsonDeleteResult = {
  message: string
  count: number
}

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
  min: JsonDouble
  max: JsonDouble
  sum: JsonDouble
  avg: JsonDouble
}
