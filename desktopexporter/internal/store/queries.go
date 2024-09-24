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
		resourceAttributes JSON,
		resourceDroppedAttributesCount INTEGER,
		scopeName VARCHAR,
		scopeVersion VARCHAR,
		scopeAttributes JSON,
		scopeDroppedAttributesCount INTEGER, 
		droppedAttributesCount INTEGER, 
		droppedEventsCount INTEGER, 
		droppedLinksCount INTEGER,
		statusCode VARCHAR, 
		statusMessage VARCHAR)
	`

	SELECT_ORDERED_TRACES = `
		SELECT DISTINCT ON(traceID) traceID,
		FROM spans
		ORDER BY startTime DESC;
	`
	SELECT_TRACE string = `
		SELECT * 
		FROM spans 
		WHERE traceID = ?
		GROUP BY parentSpanId
		ORDER BY startTime
	`

	SELECT_ROOT_SPAN string = `
		SELECT resourceAttributes.service.name, name, startTime, endTime
		FROM spans
		WHERE traceID = ?
		AND parentSpanID IS NULL 
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
