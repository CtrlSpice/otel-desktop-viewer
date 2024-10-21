package store

const (
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
		attributes JSON, 
		events JSON,
		links JSON,
		resourceAttributes JSON,
		resourceDroppedAttributesCount UINTEGER,
		scopeName VARCHAR,
		scopeVersion VARCHAR,
		scopeAttributes JSON,
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
	SELECT_TRACE string = `
		SELECT *
		FROM spans 
		WHERE traceID = ?
	`
	SELECT_ROOT_SPAN string = `
		SELECT ifnull(resourceAttributes->>'service.name', ''), name, startTime, endTime
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
	ENABLE_JSON string = `
		INSTALL json;
		LOAD json;
	`
)
