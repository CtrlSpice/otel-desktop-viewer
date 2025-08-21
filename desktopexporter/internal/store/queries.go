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
	`CREATE TYPE exemplar AS STRUCT(
		Timestamp BIGINT,
		Value DOUBLE,
		TraceID VARCHAR,
		SpanID VARCHAR,
		FilteredAttributes MAP(VARCHAR, attribute)
	)`,
	`CREATE TYPE buckets AS STRUCT(
		BucketOffset INTEGER,
		BucketCounts UBIGINT[]
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
		Timestamp BIGINT,
		StartTime BIGINT,
		Attributes MAP(VARCHAR, attribute),
		Flags UINTEGER,
		ValueType VARCHAR,
		Value DOUBLE,
		Exemplars exemplar[]
	)`,
	`CREATE TYPE sum AS STRUCT(
		Timestamp BIGINT,
		StartTime BIGINT,
		Attributes MAP(VARCHAR, attribute),
		Flags UINTEGER,
		ValueType VARCHAR,
		Value DOUBLE,
		IsMonotonic BOOLEAN,
		Exemplars exemplar[],
		AggregationTemporality VARCHAR
	)`,
	`CREATE TYPE histogram AS STRUCT(
		Timestamp BIGINT,
		StartTime BIGINT,
		Attributes MAP(VARCHAR, attribute),
		Flags UINTEGER,
		Count UBIGINT,
		Sum DOUBLE,
		Min DOUBLE,
		Max DOUBLE,
		BucketCounts UBIGINT[],
		ExplicitBounds DOUBLE[],
		Exemplars exemplar[],
		AggregationTemporality VARCHAR
	)`,
	`CREATE TYPE exponentialHistogram AS STRUCT(
		Timestamp BIGINT,
		StartTime BIGINT,
		Attributes MAP(VARCHAR, attribute),
		Flags UINTEGER,
		Count UBIGINT,
		Sum DOUBLE,
		Min DOUBLE,
		Max DOUBLE,
		Scale INTEGER,
		ZeroCount UBIGINT,
		Positive buckets,
		Negative buckets,
		Exemplars exemplar[],
		AggregationTemporality VARCHAR
	)`,
	`CREATE TYPE dataPoints AS UNION(
		Gauge gauge[],
		Sum sum[],
		Histogram histogram[],
		ExponentialHistogram exponentialHistogram[]
	)`,
	`CREATE TYPE event AS STRUCT(
		Name VARCHAR,
		Timestamp BIGINT,
		Attributes MAP(VARCHAR, attribute),
		DroppedAttributesCount UINTEGER
	)`,
	`CREATE TYPE link AS STRUCT(
		TraceID VARCHAR,
		SpanID VARCHAR,
		TraceState VARCHAR,
		Attributes MAP(VARCHAR, attribute),
		DroppedAttributesCount UINTEGER
	)`,
}

// Table creation queries that can be run in any order
var TableCreationQueries = []string{
	`CREATE TABLE IF NOT EXISTS spans 
	(TraceID VARCHAR, 
	TraceState VARCHAR, 
	SpanID VARCHAR, 
	ParentSpanID VARCHAR,
	Name VARCHAR, 
	Kind VARCHAR, 
	StartTime BIGINT, 
	EndTime BIGINT,
	Attributes MAP(VARCHAR, attribute), 
	Events event[],
	Links link[],
	ResourceAttributes MAP(VARCHAR, attribute),
	ResourceDroppedAttributesCount UINTEGER,
	ScopeName VARCHAR,
	ScopeVersion VARCHAR,
	ScopeAttributes MAP(VARCHAR, attribute),
	ScopeDroppedAttributesCount UINTEGER, 
	DroppedAttributesCount UINTEGER, 
	DroppedEventsCount UINTEGER, 
	DroppedLinksCount UINTEGER,
	StatusCode VARCHAR, 
	StatusMessage VARCHAR)`,
	`CREATE TABLE IF NOT EXISTS logs (
		LogID VARCHAR,
		Timestamp BIGINT,
		ObservedTimestamp BIGINT,
		TraceID VARCHAR,
		SpanID VARCHAR,
		SeverityText VARCHAR,
		SeverityNumber INTEGER,
		Body body,
		ResourceAttributes MAP(VARCHAR, attribute),
		ResourceDroppedAttributesCount UINTEGER,
		ScopeName VARCHAR,
		ScopeVersion VARCHAR,
		ScopeAttributes MAP(VARCHAR, attribute),
		ScopeDroppedAttributesCount UINTEGER,
		Attributes MAP(VARCHAR, attribute),
		DroppedAttributesCount UINTEGER,
		Flags UINTEGER,
		EventName VARCHAR
	)`,
	`CREATE TABLE IF NOT EXISTS metrics (
		MetricID VARCHAR,
		Name VARCHAR,
		Description VARCHAR,
		Unit VARCHAR,
		DataPoints dataPoints,
		ResourceAttributes MAP(VARCHAR, attribute),
		ResourceDroppedAttributesCount UINTEGER,
		ScopeName VARCHAR,
		ScopeVersion VARCHAR,
		ScopeAttributes MAP(VARCHAR, attribute),
		ScopeDroppedAttributesCount UINTEGER,
		Received BIGINT
	)`,
}

// Log queries
const (
	// To order, use Timestamp if present,
	// otherwise fall back to ObservedTimestamp per OpenTelemetry spec
	SelectLogs = `
		SELECT Timestamp, ObservedTimestamp, TraceID, SpanID, SeverityText, SeverityNumber,
		       Body, ResourceAttributes, ResourceDroppedAttributesCount, ScopeName, ScopeVersion,
		       ScopeAttributes, ScopeDroppedAttributesCount, Attributes, DroppedAttributesCount,
		       Flags, EventName
		FROM logs
		ORDER BY CASE 
			WHEN Timestamp IS NULL THEN ObservedTimestamp
			WHEN Timestamp = 0 THEN ObservedTimestamp
			ELSE Timestamp
		END DESC
	`

	SelectLog = `
		SELECT Timestamp, ObservedTimestamp, TraceID, SpanID, SeverityText, SeverityNumber,
		       Body, ResourceAttributes, ResourceDroppedAttributesCount, ScopeName, ScopeVersion,
		       ScopeAttributes, ScopeDroppedAttributesCount, Attributes, DroppedAttributesCount,
		       Flags, EventName
		FROM logs WHERE LogID = ?
	`

	SelectLogsByTraceSpan = `
		SELECT Timestamp, ObservedTimestamp, TraceID, SpanID, SeverityText, SeverityNumber,
		       Body, ResourceAttributes, ResourceDroppedAttributesCount, ScopeName, ScopeVersion,
		       ScopeAttributes, ScopeDroppedAttributesCount, Attributes, DroppedAttributesCount,
		       Flags, EventName
		FROM logs WHERE TraceID = ? AND SpanID = ?
	`

	SelectLogsByTrace = `
		SELECT Timestamp, ObservedTimestamp, TraceID, SpanID, SeverityText, SeverityNumber,
		       Body, ResourceAttributes, ResourceDroppedAttributesCount, ScopeName, ScopeVersion,
		       ScopeAttributes, ScopeDroppedAttributesCount, Attributes, DroppedAttributesCount,
		       Flags, EventName
		FROM logs WHERE TraceID = ?
		ORDER BY CASE 
			WHEN Timestamp IS NULL THEN ObservedTimestamp
			WHEN Timestamp = 0 THEN ObservedTimestamp
			ELSE Timestamp
		END DESC
	`
)

// Trace queries
const (
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
        SELECT DISTINCT ON (s.TraceID)
            s.TraceID,
            CASE WHEN s.ParentSpanID = '' THEN CAST(s.ResourceAttributes['service.name'] AS VARCHAR) END as service_name,
            CASE WHEN s.ParentSpanID = '' THEN s.Name END as root_name,
            CASE WHEN s.ParentSpanID = '' THEN s.StartTime END as start_time,
            CASE WHEN s.ParentSpanID = '' THEN s.EndTime END as end_time,
            COUNT(*) OVER (PARTITION BY s.TraceID) as span_count
        FROM spans s
        ORDER BY 
            COALESCE(
                MIN(CASE WHEN s.ParentSpanID = '' THEN s.StartTime END) OVER (PARTITION BY s.TraceID),
                MIN(s.StartTime) OVER (PARTITION BY s.TraceID)
            ) DESC,
            s.TraceID
    `

	// SelectTrace retrieves spans in hierarchical order with depth information
	// This mimics a tree structure that can be easily converted to a tree on the frontend
	SelectTrace = `
		WITH RECURSIVE
		-- Get the single root span (if it exists)
		root_span AS (
			SELECT 
				TraceID,
				TraceState,
				SpanID,
				ParentSpanID,
				Name,
				Kind,
				StartTime,
				EndTime,
				Attributes,
				Events,
				Links,
				ResourceAttributes,
				ResourceDroppedAttributesCount,
				ScopeName,
				ScopeVersion,
				ScopeAttributes,
				ScopeDroppedAttributesCount,
				DroppedAttributesCount,
				DroppedEventsCount,
				DroppedLinksCount,
				StatusCode,
				StatusMessage,
				0 as depth
			FROM spans 
			WHERE TraceID = ? AND (ParentSpanID = '' OR ParentSpanID IS NULL)
		),
		-- Recursively find all children of the root span
		root_children AS (
			-- Anchor: Direct children of root span
			SELECT 
				s.TraceID,
				s.TraceState,
				s.SpanID,
				s.ParentSpanID,
				s.Name,
				s.Kind,
				s.StartTime,
				s.EndTime,
				s.Attributes,
				s.Events,
				s.Links,
				s.ResourceAttributes,
				s.ResourceDroppedAttributesCount,
				s.ScopeName,
				s.ScopeVersion,
				s.ScopeAttributes,
				s.ScopeDroppedAttributesCount,
				s.DroppedAttributesCount,
				s.DroppedEventsCount,
				s.DroppedLinksCount,
				s.StatusCode,
				s.StatusMessage,
				1 as depth
			FROM spans s
			JOIN root_span r ON s.ParentSpanID = r.SpanID AND s.TraceID = r.TraceID
			WHERE s.TraceID = ?
			
			UNION ALL
			
			-- Recursive: Find children of children
			SELECT 
				s.TraceID,
				s.TraceState,
				s.SpanID,
				s.ParentSpanID,
				s.Name,
				s.Kind,
				s.StartTime,
				s.EndTime,
				s.Attributes,
				s.Events,
				s.Links,
				s.ResourceAttributes,
				s.ResourceDroppedAttributesCount,
				s.ScopeName,
				s.ScopeVersion,
				s.ScopeAttributes,
				s.ScopeDroppedAttributesCount,
				s.DroppedAttributesCount,
				s.DroppedEventsCount,
				s.DroppedLinksCount,
				s.StatusCode,
				s.StatusMessage,
				rc.depth + 1
			FROM spans s
			JOIN root_children rc ON s.ParentSpanID = rc.SpanID AND s.TraceID = rc.TraceID
			WHERE s.TraceID = ?
		),
		-- Find orphan spans (have parent but parent not in our data)
		orphan_spans AS (
			SELECT DISTINCT s.ParentSpanID as spanID
			FROM spans s
			WHERE s.TraceID = ? 
				AND s.ParentSpanID != ''
				AND s.ParentSpanID NOT IN (SELECT SpanID FROM spans WHERE TraceID = ?)
		),
		-- Build orphan tree with their children recursively
		orphan_tree AS (
			-- Anchor: Start with orphan spans (spans whose parents don't exist)
			SELECT 
				TraceID,
				TraceState,
				SpanID,
				ParentSpanID,
				Name,
				Kind,
				StartTime,
				EndTime,
				Attributes,
				Events,
				Links,
				ResourceAttributes,
				ResourceDroppedAttributesCount,
				ScopeName,
				ScopeVersion,
				ScopeAttributes,
				ScopeDroppedAttributesCount,
				DroppedAttributesCount,
				DroppedEventsCount,
				DroppedLinksCount,
				StatusCode,
				StatusMessage,
				0 as depth
			FROM spans 
			WHERE TraceID = ?
				AND ParentSpanID != ''
				AND ParentSpanID NOT IN (SELECT SpanID FROM spans WHERE TraceID = ?)
			
			UNION ALL
			
			-- Recursive: Find children of orphan spans
			SELECT 
				s.TraceID,
				s.TraceState,
				s.SpanID,
				s.ParentSpanID,
				s.Name,
				s.Kind,
				s.StartTime,
				s.EndTime,
				s.Attributes,
				s.Events,
				s.Links,
				s.ResourceAttributes,
				s.ResourceDroppedAttributesCount,
				s.ScopeName,
				s.ScopeVersion,
				s.ScopeAttributes,
				s.ScopeDroppedAttributesCount,
				s.DroppedAttributesCount,
				s.DroppedEventsCount,
				s.DroppedLinksCount,
				s.StatusCode,
				s.StatusMessage,
				ot.depth + 1
			FROM spans s
			JOIN orphan_tree ot ON s.ParentSpanID = ot.SpanID AND s.TraceID = ot.TraceID
			WHERE s.TraceID = ?
		)
		-- Return all spans in hierarchical order
		SELECT 
			TraceID,
			TraceState,
			SpanID,
			ParentSpanID,
			Name,
			Kind,
			StartTime,
			EndTime,
			Attributes,
			Events,
			Links,
			ResourceAttributes,
			ResourceDroppedAttributesCount,
			ScopeName,
			ScopeVersion,
			ScopeAttributes,
			ScopeDroppedAttributesCount,
			DroppedAttributesCount,
			DroppedEventsCount,
			DroppedLinksCount,
			StatusCode,
			StatusMessage,
			depth
		FROM root_span
		
		UNION ALL
		
		-- Add children of root span
		SELECT 
			TraceID,
			TraceState,
			SpanID,
			ParentSpanID,
			Name,
			Kind,
			StartTime,
			EndTime,
			Attributes,
			Events,
			Links,
			ResourceAttributes,
			ResourceDroppedAttributesCount,
			ScopeName,
			ScopeVersion,
			ScopeAttributes,
			ScopeDroppedAttributesCount,
			DroppedAttributesCount,
			DroppedEventsCount,
			DroppedLinksCount,
			StatusCode,
			StatusMessage,
			depth
		FROM root_children
		
		UNION ALL
		
		-- Add orphan spans with their children
		SELECT 
			TraceID,
			TraceState,
			SpanID,
			ParentSpanID,
			Name,
			Kind,
			StartTime,
			EndTime,
			Attributes,
			Events,
			Links,
			ResourceAttributes,
			ResourceDroppedAttributesCount,
			ScopeName,
			ScopeVersion,
			ScopeAttributes,
			ScopeDroppedAttributesCount,
			DroppedAttributesCount,
			DroppedEventsCount,
			DroppedLinksCount,
			StatusCode,
			StatusMessage,
			depth
		FROM orphan_tree
		
		ORDER BY depth, StartTime
	`
)

// Metrics queries
const (
	SelectMetrics = `
		SELECT Name, Description, Unit, DataPoints, ResourceAttributes, 
		       ResourceDroppedAttributesCount, ScopeName, ScopeVersion, ScopeAttributes,
		       ScopeDroppedAttributesCount, Received
		FROM metrics
		ORDER BY Received DESC
	`

	SelectMetric = `
		SELECT Name, Description, Unit, DataPoints, ResourceAttributes, 
		       ResourceDroppedAttributesCount, ScopeName, ScopeVersion, ScopeAttributes,
		       ScopeDroppedAttributesCount, Received
		FROM metrics WHERE MetricID = ?
	`
)

// Maintenance queries
const (
	TruncateSpans   = `TRUNCATE TABLE spans`
	TruncateLogs    = `TRUNCATE TABLE logs`
	TruncateMetrics = `TRUNCATE TABLE metrics`
)
