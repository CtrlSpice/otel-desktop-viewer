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
	CREATE_ROOT_SPAN_MACRO = `
        CREATE MACRO get_root_span_field(trace_id, field) AS (
            SELECT field
            FROM spans
            WHERE traceID = trace_id
            AND parentSpanID = ''
            LIMIT 1
        );
    `
	CREATE_HAS_ROOT_SPAN_MACRO = `
		CREATE MACRO has_root_span(trace_id) AS (
			SELECT EXISTS (
				SELECT 1
				FROM spans
				WHERE traceID = trace_id
				AND parentSpanID = ''
			)
		);
	`
	SELECT_TRACE_SUMMARIES = `
        SELECT 
            t.traceID,
            has_root_span(t.traceID) as hasRootSpan,
            CASE 
                WHEN has_root_span(t.traceID) THEN get_root_span_field(t.traceID, CAST(UNNEST(resourceAttributes['service.name']) AS VARCHAR))
                ELSE ''
            END as serviceName,
            CASE 
                WHEN has_root_span(t.traceID) THEN get_root_span_field(t.traceID, name)
                ELSE ''
            END as name,
            CASE 
                WHEN has_root_span(t.traceID) THEN get_root_span_field(t.traceID, startTime)
                ELSE 0::TIMESTAMP_NS
            END as startTime,
            CASE 
                WHEN has_root_span(t.traceID) THEN get_root_span_field(t.traceID, endTime)
                ELSE 0::TIMESTAMP_NS
            END as endTime,
            (SELECT count(*) FROM spans WHERE traceID = t.traceID) AS spanCount
        FROM (SELECT DISTINCT traceID FROM spans) t
        ORDER BY (SELECT MAX(startTime) FROM spans WHERE traceID = t.traceID) DESC
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
	TRUNCATE_SPANS string = `
		TRUNCATE spans;
	`
)
