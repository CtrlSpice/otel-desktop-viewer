package store

const (
	// An Attribute is a key-value pair, which MUST have the following properties:

	// The attribute key MUST be a non-null and non-empty string.
	// Case sensitivity of keys is preserved. Keys that differ in casing are treated as distinct keys.
	// The attribute value is either:
	// A primitive type: string, boolean, double precision floating point (IEEE 754-1985) or signed 64 bit integer.
	// An array of primitive type values. The array MUST be homogeneous, i.e., it MUST NOT contain values of different types.
	CREATE_ATTRIBUTE_TYPE string = `
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
	CREATE_EVENT_TYPE string = `
		CREATE TYPE event AS STRUCT(
			name VARCHAR,
			timestamp TIMESTAMP_NS,
			attributes MAP(VARCHAR, attribute),
			droppedAttributesCount UINTEGER
		)
	`
	CREATE_LINK_TYPE string = `
		CREATE TYPE link AS STRUCT(
			traceID VARCHAR,
			spanID VARCHAR,
			traceState VARCHAR,
			attributes MAP(VARCHAR, attribute),
			droppedAttributesCount UINTEGER
		)
	`
	CREATE_SPANS_TABLE string = `
		CREATE TABLE IF NOT EXISTS spans 
		(traceID VARCHAR, 
		traceState VARCHAR, 
		spanID VARCHAR, 
		parentSpanID VARCHAR,
		name VARCHAR, 
		kind VARCHAR, 
		startTime TIMESTAMP_NS, 
		endTime TIMESTAMP_NS,
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
	SELECT_ORDERED_TRACES = `
		SELECT traceID 
		FROM spans
		GROUP BY traceID
		ORDER BY MAX(startTime) DESC
	`

	// DuckDB's Go bindings have limited support for complex types like UNIONs and STRUCTs
	// So we need to cast the attributes to VARCHAR and then parse them back into the original type
	SELECT_TRACE string = `
		SELECT 
			traceID, 
			traceState, 
			spanID, 
			parentSpanID, 
			name, 
			kind, 
			startTime, 
			endTime,
			attributes::VARCHAR,
			events::VARCHAR,
			links::VARCHAR,
			resourceAttributes::VARCHAR,
			resourceDroppedAttributesCount,
			scopeName,
			scopeVersion,
			scopeAttributes::VARCHAR,
			scopeDroppedAttributesCount,
			droppedAttributesCount,
			droppedEventsCount,
			droppedLinksCount,
			statusCode,
			statusMessage
		FROM spans 
		WHERE traceID = ?
	`

	SELECT_ROOT_SPAN string = `
		SELECT 
			CAST(UNNEST(resourceAttributes['service.name']) AS VARCHAR),
			name,
			startTime,
			endTime
		FROM spans
		WHERE traceID = ?
		AND parentSpanID = '' 
	`
	SELECT_SPAN_COUNT string = `
		SELECT count(*) 
		FROM spans
		WHERE traceID = ?
	`

	TRUNCATE_SPANS string = `
		TRUNCATE spans;
	`
)
