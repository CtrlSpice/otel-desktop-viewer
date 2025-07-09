package store

// Type creation queries that must be run in order
var TypeCreationQueries = []string{
	`CREATE TYPE attribute AS UNION(
		str VARCHAR,
		bigint BIGINT,
		double DOUBLE,
		boolean BOOLEAN,
		str_list VARCHAR[],
		bigint_list BIGINT[],
		double_list DOUBLE[],
		boolean_list BOOLEAN[]
	)`,
	`CREATE TYPE metricType AS ENUM(
		'Empty', 
		'Gauge', 
		'Sum', 
		'Histogram', 
		'ExponentialHistogram'
	)`,
	`CREATE TYPE exemplar AS STRUCT(
		timestamp BIGINT,
		value DOUBLE,
		traceID VARCHAR,
		spanID VARCHAR,
		filteredAttributes MAP(VARCHAR, attribute)
	)`,
	`CREATE TYPE buckets AS STRUCT(
		offset INTEGER,
		bucketCounts UBIGINT[]
	)`,
	`CREATE TYPE body AS UNION(
		str VARCHAR,
		bigint BIGINT,
		double DOUBLE,
		boolean BOOLEAN,
		bytes BLOB,
		json JSON
	)`,
	`CREATE TYPE gauge AS STRUCT(
		timestamp BIGINT,
		startTime BIGINT,
		attributes MAP(VARCHAR, attribute),
		flags UINTEGER,
		valueType VARCHAR,
		value DOUBLE,
		exemplars exemplar[],
	)`,
	`CREATE TYPE sum AS STRUCT(
		timestamp BIGINT,
		startTime BIGINT,
		attributes MAP(VARCHAR, attribute),
		flags UINTEGER,
		valueType VARCHAR,
		value DOUBLE,
		isMonotonic BOOLEAN,
		exemplars exemplar[],
		aggregationTemporality VARCHAR,
	)`,
	`CREATE TYPE histogram AS STRUCT(
		timestamp BIGINT,
		startTime BIGINT,
		attributes MAP(VARCHAR, attribute),
		flags UINTEGER,
		count UBIGINT,
		sum DOUBLE,
		min DOUBLE,
		max DOUBLE,
		bucketCounts UBIGINT[],
		explicitBounds DOUBLE[],
		exemplars exemplar[],
		aggregationTemporality VARCHAR,
	)`,
	`CREATE TYPE exponential_histogram AS STRUCT(
		timestamp BIGINT,
		startTime BIGINT,
		attributes MAP(VARCHAR, attribute),
		flags UINTEGER,
		count UBIGINT,
		sum DOUBLE,
		min DOUBLE,
		max DOUBLE,
		scale INTEGER,
		zeroCount UBIGINT,
		positive buckets,
		negative buckets,
		exemplars exemplar[],
		aggregationTemporality VARCHAR,
	)`,
	`CREATE TYPE dataPoint AS UNION(
		Gauge gauge,
		Sum sum,
		Histogram histogram,
		ExponentialHistogram exponentialHistogram,
	)`,
	`CREATE TYPE event AS STRUCT(
		name VARCHAR,
		timestamp BIGINT,
		attributes MAP(VARCHAR, attribute),
		droppedAttributesCount UINTEGER
	)`,
	`CREATE TYPE link AS STRUCT(
		traceID VARCHAR,
		spanID VARCHAR,
		traceState VARCHAR,
		attributes MAP(VARCHAR, attribute),
		droppedAttributesCount UINTEGER
	)`,
}

// Table creation queries that can be run in any order
var TableCreationQueries = []string{
	`CREATE TABLE IF NOT EXISTS spans 
	(traceID VARCHAR, 
	traceState VARCHAR, 
	spanID VARCHAR, 
	parentSpanID VARCHAR,
	name VARCHAR, 
	kind VARCHAR, 
	startTime BIGINT, 
	endTime BIGINT,
	attributes MAP(VARCHAR, attribute), 
	events event[],
	links link[],
	resourceAttributes MAP(VARCHAR, attribute),
	resourceDroppedAttributesCount UINTEGER,
	scopeName VARCHAR,
	scopeVersion VARCHAR,
	scopeAttributes MAP(VARCHAR, attribute),
	scopeDroppedAttributesCount UINTEGER, 
	droppedAttributesCount UINTEGER, 
	droppedEventsCount UINTEGER, 
	droppedLinksCount UINTEGER,
	statusCode VARCHAR, 
	statusMessage VARCHAR)`,
	`CREATE TABLE IF NOT EXISTS logs (
		logID VARCHAR,
		timestamp BIGINT,
		observedTimestamp BIGINT,
		traceID VARCHAR,
		spanID VARCHAR,
		severityText VARCHAR,
		severityNumber INTEGER,
		body body,
		resourceAttributes MAP(VARCHAR, attribute),
		resourceDroppedAttributesCount UINTEGER,
		scopeName VARCHAR,
		scopeVersion VARCHAR,
		scopeAttributes MAP(VARCHAR, attribute),
		scopeDroppedAttributesCount UINTEGER,
		attributes MAP(VARCHAR, attribute),
		droppedAttributesCount UINTEGER,
		flags UINTEGER,
		eventName VARCHAR
	)`,
	`CREATE TABLE IF NOT EXISTS metrics (
		metricID VARCHAR,
		name VARCHAR,
		description VARCHAR,
		unit VARCHAR,
		type MetricType,
		dataPoints dataPoint[],
		resourceAttributes MAP(VARCHAR, attribute),
		resourceDroppedAttributesCount UINTEGER,
		scopeName VARCHAR,
		scopeVersion VARCHAR,
		scopeAttributes MAP(VARCHAR, attribute),
		scopeDroppedAttributesCount UINTEGER,
		received BIGINT
	)`,
}

// Log queries
const (
	// To order, use Timestamp if present,
	// otherwise fall back to ObservedTimestamp per OpenTelemetry spec
	SelectLogs = `
		SELECT timestamp, observedTimestamp, traceID, spanID, severityText, severityNumber,
		       body, resourceAttributes, resourceDroppedAttributesCount, scopeName, scopeVersion,
		       scopeAttributes, scopeDroppedAttributesCount, attributes, droppedAttributesCount,
		       flags, eventName
		FROM logs
		ORDER BY CASE 
			WHEN timestamp IS NULL THEN observedTimestamp
			WHEN timestamp = 0 THEN observedTimestamp
			ELSE timestamp
		END DESC
	`

	SelectLog = `
		SELECT timestamp, observedTimestamp, traceID, spanID, severityText, severityNumber,
		       body, resourceAttributes, resourceDroppedAttributesCount, scopeName, scopeVersion,
		       scopeAttributes, scopeDroppedAttributesCount, attributes, droppedAttributesCount,
		       flags, eventName
		FROM logs WHERE logID = ?
	`

	SelectLogsByTraceSpan = `
		SELECT timestamp, observedTimestamp, traceID, spanID, severityText, severityNumber,
		       body, resourceAttributes, resourceDroppedAttributesCount, scopeName, scopeVersion,
		       scopeAttributes, scopeDroppedAttributesCount, attributes, droppedAttributesCount,
		       flags, eventName
		FROM logs WHERE traceID = ? AND spanID = ?
	`

	SelectLogsByTrace = `
		SELECT timestamp, observedTimestamp, traceID, spanID, severityText, severityNumber,
		       body, resourceAttributes, resourceDroppedAttributesCount, scopeName, scopeVersion,
		       scopeAttributes, scopeDroppedAttributesCount, attributes, droppedAttributesCount,
		       flags, eventName
		FROM logs WHERE traceID = ?
		ORDER BY CASE 
			WHEN timestamp IS NULL THEN observedTimestamp
			WHEN timestamp = 0 THEN observedTimestamp
			ELSE timestamp
		END DESC
	`
)

// Trace queries
const (
	SelectTrace = `
		SELECT * FROM spans WHERE traceID = ?
	`

	// SelectTraceSummaries retrieves all traces ordered by:
	// - Root span start time when available
	// - Earliest span start time when no root span exists
	//
	// The ordering uses:
	//   COALESCE(
	//     MIN(CASE WHEN parentSpanID = '' THEN startTime END),  -- First try: root span time (only one per trace)
	//     MIN(startTime)                                         -- Fallback: earliest span time in trace
	//   )
	// Both MIN() are used with OVER (PARTITION BY traceID) to get times within each trace.
	// We use MIN for both because even though there's only one root span time per trace (when it exists),
	// we need an aggregate function to match the MIN used in the fallback.
	SelectTraceSummaries = `
        SELECT DISTINCT ON (s.traceID)
            s.traceID,
            CASE WHEN s.parentSpanID = '' THEN CAST(s.resourceAttributes['service.name'] AS VARCHAR) END as service_name,
            CASE WHEN s.parentSpanID = '' THEN s.name END as root_name,
            CASE WHEN s.parentSpanID = '' THEN s.startTime END as start_time,
            CASE WHEN s.parentSpanID = '' THEN s.endTime END as end_time,
            COUNT(*) OVER (PARTITION BY s.traceID) as span_count
        FROM spans s
        ORDER BY 
            COALESCE(
                MIN(CASE WHEN s.parentSpanID = '' THEN s.startTime END) OVER (PARTITION BY s.traceID),
                MIN(s.startTime) OVER (PARTITION BY s.traceID)
            ) DESC,
            s.traceID
    `
)

// Maintenance queries
const (
	TruncateSpans   = `TRUNCATE TABLE spans`
	TruncateLogs    = `TRUNCATE TABLE logs`
	TruncateMetrics = `TRUNCATE TABLE metrics`
)

// Metrics queries
const (
	SelectMetrics = `
		SELECT name, description, unit, type, dataPoints, resourceAttributes, 
		       resourceDroppedAttributesCount, scopeName, scopeVersion, scopeAttributes,
		       scopeDroppedAttributesCount, received
		FROM metrics
		ORDER BY received DESC
	`

	SelectMetric = `
		SELECT name, description, unit, type, dataPoints, resourceAttributes, 
		       resourceDroppedAttributesCount, scopeName, scopeVersion, scopeAttributes,
		       scopeDroppedAttributesCount, received
		FROM metrics WHERE metricID = ?
	`
)
