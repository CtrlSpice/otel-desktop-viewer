package store

// Table creation queries
const (
	// An Attribute is a key-value pair, which MUST have the following properties:
	// - The attribute key MUST be a non-null and non-empty string.
	// - Case sensitivity of keys is preserved. Keys that differ in casing are treated as distinct keys.
	// - The attribute value is either:
	//   - A primitive type: string, boolean, double precision floating point (IEEE 754-1985) or signed 64 bit integer.
	//   - An array of primitive type values. The array MUST be homogeneous, i.e., it MUST NOT contain values of different types.
	CreateAttributeType = `
		CREATE TYPE attribute AS UNION(
			str VARCHAR,
			bigint BIGINT,
			double DOUBLE,
			boolean BOOLEAN,
			str_list VARCHAR[],
			bigint_list BIGINT[],
			double_list DOUBLE[],
			boolean_list BOOLEAN[]
		)
	`
	// BodyType supports all value types according to semantic conventions:
	// - Scalar values: string, boolean, signed 64-bit integer, double
	// - Byte array
	// - Everything else (arrays, maps, etc.) as JSON
	CreateBodyType = `
		CREATE TYPE body AS UNION(
			str VARCHAR,
			bigint BIGINT,
			double DOUBLE,
			boolean BOOLEAN,
			bytes BLOB,
			json JSON
		)
	`
	CreateEventType = `
		CREATE TYPE event AS STRUCT(
			name VARCHAR,
			timestamp BIGINT,
			attributes MAP(VARCHAR, attribute),
			droppedAttributesCount UINTEGER
		)
	`
	CreateLinkType = `
		CREATE TYPE link AS STRUCT(
			traceID VARCHAR,
			spanID VARCHAR,
			traceState VARCHAR,
			attributes MAP(VARCHAR, attribute),
			droppedAttributesCount UINTEGER
		)
	`
	CreateSpansTable = `
		CREATE TABLE IF NOT EXISTS spans 
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
		statusMessage VARCHAR)
	`
	CreateLogsTable = `
		CREATE TABLE IF NOT EXISTS logs (
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
		)	`
)

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
	TruncateSpans = `TRUNCATE TABLE spans`
	TruncateLogs = `TRUNCATE TABLE logs`
)