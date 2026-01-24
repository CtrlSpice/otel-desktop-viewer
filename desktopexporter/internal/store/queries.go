package store

import "strings"

// Type creation queries
var TypeCreationQueries = []string{
	`CREATE TYPE attr_type AS ENUM('string', 'int64', 'float64', 'bool', 'string[]', 'int64[]', 'float64[]', 'boolean[]')`,
}

// Table creation queries
// Order matters: spans before events/links, metrics before datapoints, datapoints before exemplars (FK dependencies)
var TableCreationQueries = []string{
	`CREATE TABLE IF NOT EXISTS spans (
		TraceID VARCHAR,
		TraceState VARCHAR,
		SpanID VARCHAR PRIMARY KEY,
		ParentSpanID VARCHAR,
		Name VARCHAR,
		Kind VARCHAR,
		StartTime BIGINT,
		EndTime BIGINT,
		ResourceDroppedAttributesCount UINTEGER,
		ScopeName VARCHAR,
		ScopeVersion VARCHAR,
		ScopeDroppedAttributesCount UINTEGER,
		DroppedAttributesCount UINTEGER,
		DroppedEventsCount UINTEGER,
		DroppedLinksCount UINTEGER,
		StatusCode VARCHAR,
		StatusMessage VARCHAR
	)`,
	`CREATE TABLE IF NOT EXISTS events (
		EventID VARCHAR PRIMARY KEY DEFAULT gen_random_uuid()::VARCHAR,
		SpanID VARCHAR NOT NULL,
		Name VARCHAR,
		Timestamp BIGINT,
		DroppedAttributesCount UINTEGER,
		FOREIGN KEY (SpanID) REFERENCES spans(SpanID) ON DELETE CASCADE
	)`,
	`CREATE TABLE IF NOT EXISTS links (
		LinkID VARCHAR PRIMARY KEY DEFAULT gen_random_uuid()::VARCHAR,
		SpanID VARCHAR NOT NULL,
		TraceID VARCHAR,
		LinkedSpanID VARCHAR,
		TraceState VARCHAR,
		DroppedAttributesCount UINTEGER,
		FOREIGN KEY (SpanID) REFERENCES spans(SpanID) ON DELETE CASCADE
	)`,
	`CREATE TABLE IF NOT EXISTS logs (
		ID VARCHAR PRIMARY KEY DEFAULT gen_random_uuid()::VARCHAR,
		Timestamp BIGINT,
		ObservedTimestamp BIGINT,
		TraceID VARCHAR,
		SpanID VARCHAR,
		SeverityText VARCHAR,
		SeverityNumber INTEGER,
		Body VARCHAR,
		BodyType VARCHAR,
		ResourceDroppedAttributesCount UINTEGER,
		ScopeName VARCHAR,
		ScopeVersion VARCHAR,
		ScopeDroppedAttributesCount UINTEGER,
		DroppedAttributesCount UINTEGER,
		Flags UINTEGER,
		EventName VARCHAR
	)`,
	`CREATE TABLE IF NOT EXISTS metrics (
		ID VARCHAR PRIMARY KEY DEFAULT gen_random_uuid()::VARCHAR,
		Name VARCHAR,
		Description VARCHAR,
		Unit VARCHAR,
		ResourceDroppedAttributesCount UINTEGER,
		ScopeName VARCHAR,
		ScopeVersion VARCHAR,
		ScopeDroppedAttributesCount UINTEGER,
		Received BIGINT
	)`,
	`CREATE TABLE IF NOT EXISTS datapoints (
		ID VARCHAR PRIMARY KEY DEFAULT gen_random_uuid()::VARCHAR,
		MetricID VARCHAR NOT NULL,
		MetricType VARCHAR NOT NULL,
		Timestamp BIGINT,
		StartTime BIGINT,
		Flags UINTEGER,
		Value DOUBLE,
		ValueType VARCHAR,
		IsMonotonic BOOLEAN,
		AggregationTemporality VARCHAR,
		Count UBIGINT,
		Sum DOUBLE,
		Min DOUBLE,
		Max DOUBLE,
		BucketCounts UBIGINT[],
		ExplicitBounds DOUBLE[],
		Scale INTEGER,
		ZeroCount UBIGINT,
		PositiveBucketOffset INTEGER,
		PositiveBucketCounts UBIGINT[],
		NegativeBucketOffset INTEGER,
		NegativeBucketCounts UBIGINT[],
		FOREIGN KEY (MetricID) REFERENCES metrics(ID) ON DELETE CASCADE
	)`,
	`CREATE TABLE IF NOT EXISTS exemplars (
		ID VARCHAR PRIMARY KEY DEFAULT gen_random_uuid()::VARCHAR,
		DataPointID VARCHAR NOT NULL,
		Timestamp BIGINT,
		Value DOUBLE,
		TraceID VARCHAR,
		SpanID VARCHAR,
		FOREIGN KEY (DataPointID) REFERENCES datapoints(ID) ON DELETE CASCADE
	)`,
	`CREATE TABLE IF NOT EXISTS attributes (
		-- ID columns (only relevant ones populated per row based on attribute scope)
		-- For span/resource/scope attributes: SpanID only
		-- For event attributes: EventID (direct), SpanID (parent)
		-- For link attributes: LinkID (direct), SpanID (parent)
		-- For log/resource/scope attributes: LogID only (references logs.ID)
		-- For metric/resource/scope attributes: MetricID only (references metrics.ID)
		-- For data_point attributes: DataPointID (direct), MetricID (parent)
		-- For exemplar attributes: ExemplarID (direct), DataPointID (parent), MetricID (grandparent)
		SpanID VARCHAR,
		EventID VARCHAR,
		LinkID VARCHAR,
		LogID VARCHAR,
		MetricID VARCHAR,
		DataPointID VARCHAR,
		ExemplarID VARCHAR,
		-- Attribute data
		Key VARCHAR NOT NULL,
		Value VARCHAR NOT NULL,
		Type attr_type NOT NULL,
		-- Foreign keys
		FOREIGN KEY (SpanID) REFERENCES spans(SpanID) ON DELETE CASCADE,
		FOREIGN KEY (EventID) REFERENCES events(EventID) ON DELETE CASCADE,
		FOREIGN KEY (LinkID) REFERENCES links(LinkID) ON DELETE CASCADE,
		FOREIGN KEY (LogID) REFERENCES logs(ID) ON DELETE CASCADE,
		FOREIGN KEY (MetricID) REFERENCES metrics(ID) ON DELETE CASCADE,
		FOREIGN KEY (DataPointID) REFERENCES datapoints(ID) ON DELETE CASCADE,
		FOREIGN KEY (ExemplarID) REFERENCES exemplars(ID) ON DELETE CASCADE,
		-- Unique constraint: combination of all ID columns + Key ensures uniqueness
		-- (NULLs are excluded from uniqueness checks, so only populated IDs matter)
		UNIQUE (SpanID, EventID, LinkID, LogID, MetricID, DataPointID, ExemplarID, Key)
	)`,
}

// Constraint creation queries for discriminated union enforcement
// Note: All datapoints for a given MetricID must have the same MetricType (enforced at application level)
var ConstraintCreationQueries = []string{
	// Attributes table: Ensure exactly one direct owner ID is populated and parent IDs are correct
	`ALTER TABLE attributes ADD CONSTRAINT chk_attributes_one_owner CHECK (
		-- Span attributes: SpanID only
		(SpanID IS NOT NULL AND EventID IS NULL AND LinkID IS NULL AND LogID IS NULL AND MetricID IS NULL AND DataPointID IS NULL AND ExemplarID IS NULL) OR
		-- Event attributes: EventID (direct) and SpanID (parent)
		(EventID IS NOT NULL AND SpanID IS NOT NULL AND LinkID IS NULL AND LogID IS NULL AND MetricID IS NULL AND DataPointID IS NULL AND ExemplarID IS NULL) OR
		-- Link attributes: LinkID (direct) and SpanID (parent)
		(LinkID IS NOT NULL AND SpanID IS NOT NULL AND EventID IS NULL AND LogID IS NULL AND MetricID IS NULL AND DataPointID IS NULL AND ExemplarID IS NULL) OR
		-- Log attributes: LogID only
		(LogID IS NOT NULL AND SpanID IS NULL AND EventID IS NULL AND LinkID IS NULL AND MetricID IS NULL AND DataPointID IS NULL AND ExemplarID IS NULL) OR
		-- Metric attributes: MetricID only
		(MetricID IS NOT NULL AND SpanID IS NULL AND EventID IS NULL AND LinkID IS NULL AND LogID IS NULL AND DataPointID IS NULL AND ExemplarID IS NULL) OR
		-- Data point attributes: DataPointID (direct) and MetricID (parent)
		(DataPointID IS NOT NULL AND MetricID IS NOT NULL AND SpanID IS NULL AND EventID IS NULL AND LinkID IS NULL AND LogID IS NULL AND ExemplarID IS NULL) OR
		-- Exemplar attributes: ExemplarID (direct), DataPointID (parent), MetricID (grandparent)
		(ExemplarID IS NOT NULL AND DataPointID IS NOT NULL AND MetricID IS NOT NULL AND SpanID IS NULL AND EventID IS NULL AND LinkID IS NULL AND LogID IS NULL)
	)`,
	// Ensure MetricType is one of the valid values
	`ALTER TABLE datapoints ADD CONSTRAINT chk_metric_type_valid CHECK (
		MetricType IN ('Gauge', 'Sum', 'Histogram', 'ExponentialHistogram')
	)`,
	// Enforce Gauge type: Value and ValueType must be NOT NULL, histogram fields must be NULL
	`ALTER TABLE datapoints ADD CONSTRAINT chk_gauge_fields CHECK (
		(MetricType != 'Gauge') OR (
			Value IS NOT NULL AND ValueType IS NOT NULL AND
			Count IS NULL AND Sum IS NULL AND Min IS NULL AND Max IS NULL AND
			BucketCounts IS NULL AND ExplicitBounds IS NULL AND
			Scale IS NULL AND ZeroCount IS NULL AND
			PositiveBucketOffset IS NULL AND PositiveBucketCounts IS NULL AND
			NegativeBucketOffset IS NULL AND NegativeBucketCounts IS NULL AND
			AggregationTemporality IS NULL
		)
	)`,
	// Enforce Sum type: Value, ValueType, IsMonotonic, AggregationTemporality must be NOT NULL, histogram fields must be NULL
	`ALTER TABLE datapoints ADD CONSTRAINT chk_sum_fields CHECK (
		(MetricType != 'Sum') OR (
			Value IS NOT NULL AND ValueType IS NOT NULL AND
			IsMonotonic IS NOT NULL AND AggregationTemporality IS NOT NULL AND
			Count IS NULL AND Sum IS NULL AND Min IS NULL AND Max IS NULL AND
			BucketCounts IS NULL AND ExplicitBounds IS NULL AND
			Scale IS NULL AND ZeroCount IS NULL AND
			PositiveBucketOffset IS NULL AND PositiveBucketCounts IS NULL AND
			NegativeBucketOffset IS NULL AND NegativeBucketCounts IS NULL
		)
	)`,
	// Enforce Histogram type: Count, Sum, BucketCounts, ExplicitBounds, AggregationTemporality must be NOT NULL, gauge/sum fields must be NULL
	// Note: Min and Max are optional in OpenTelemetry, so they can be NULL
	`ALTER TABLE datapoints ADD CONSTRAINT chk_histogram_fields CHECK (
		(MetricType != 'Histogram') OR (
			Count IS NOT NULL AND Sum IS NOT NULL AND
			BucketCounts IS NOT NULL AND ExplicitBounds IS NOT NULL AND
			AggregationTemporality IS NOT NULL AND
			Value IS NULL AND ValueType IS NULL AND IsMonotonic IS NULL AND
			Scale IS NULL AND ZeroCount IS NULL AND
			PositiveBucketOffset IS NULL AND PositiveBucketCounts IS NULL AND
			NegativeBucketOffset IS NULL AND NegativeBucketCounts IS NULL
		)
	)`,
	// Enforce ExponentialHistogram type: Count, Sum, Scale, ZeroCount, bucket fields, AggregationTemporality must be NOT NULL, other fields must be NULL
	// Note: Min and Max are optional in OpenTelemetry, so they can be NULL
	`ALTER TABLE datapoints ADD CONSTRAINT chk_exponential_histogram_fields CHECK (
		(MetricType != 'ExponentialHistogram') OR (
			Count IS NOT NULL AND Sum IS NOT NULL AND
			Scale IS NOT NULL AND ZeroCount IS NOT NULL AND
			PositiveBucketOffset IS NOT NULL AND PositiveBucketCounts IS NOT NULL AND
			NegativeBucketOffset IS NOT NULL AND NegativeBucketCounts IS NOT NULL AND
			AggregationTemporality IS NOT NULL AND
			Value IS NULL AND ValueType IS NULL AND IsMonotonic IS NULL AND
			BucketCounts IS NULL AND ExplicitBounds IS NULL
		)
	)`,
}

// Index creation queries
var IndexCreationQueries = []string{
	`CREATE INDEX IF NOT EXISTS idx_spans_traceid ON spans(TraceID)`,
	`CREATE INDEX IF NOT EXISTS idx_spans_starttime ON spans(StartTime)`,
	`CREATE INDEX IF NOT EXISTS idx_spans_parentspanid ON spans(ParentSpanID)`,
	`CREATE INDEX IF NOT EXISTS idx_events_span ON events(SpanID)`,
	`CREATE INDEX IF NOT EXISTS idx_events_timestamp ON events(Timestamp)`,
	`CREATE INDEX IF NOT EXISTS idx_links_span ON links(SpanID)`,
	`CREATE INDEX IF NOT EXISTS idx_links_trace ON links(TraceID, LinkedSpanID)`,
	`CREATE INDEX IF NOT EXISTS idx_logs_timestamp ON logs(Timestamp)`,
	`CREATE INDEX IF NOT EXISTS idx_logs_traceid ON logs(TraceID)`,
	`CREATE INDEX IF NOT EXISTS idx_logs_severitynumber ON logs(SeverityNumber)`,
	`CREATE INDEX IF NOT EXISTS idx_metrics_name ON metrics(Name)`,
	`CREATE INDEX IF NOT EXISTS idx_metrics_received ON metrics(Received)`,
	`CREATE INDEX IF NOT EXISTS idx_datapoints_type_metric_time ON datapoints(MetricType, MetricID, Timestamp DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_datapoints_metric_time ON datapoints(MetricID, Timestamp DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_datapoints_time ON datapoints(Timestamp DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_exemplars_datapoint ON exemplars(DataPointID)`,
	`CREATE INDEX IF NOT EXISTS idx_exemplars_trace ON exemplars(TraceID, SpanID)`,
	-- Direct entity lookups (covering Key, Value, Type for common queries)
	`CREATE INDEX IF NOT EXISTS idx_attributes_span ON attributes(SpanID, Key, Value, Type)`,
	`CREATE INDEX IF NOT EXISTS idx_attributes_event ON attributes(EventID, Key, Value, Type)`,
	`CREATE INDEX IF NOT EXISTS idx_attributes_link ON attributes(LinkID, Key, Value, Type)`,
	`CREATE INDEX IF NOT EXISTS idx_attributes_log ON attributes(LogID, Key, Value, Type)`,
	`CREATE INDEX IF NOT EXISTS idx_attributes_metric ON attributes(MetricID, Key, Value, Type)`,
	`CREATE INDEX IF NOT EXISTS idx_attributes_datapoint ON attributes(DataPointID, Key, Value, Type)`,
	`CREATE INDEX IF NOT EXISTS idx_attributes_exemplar ON attributes(ExemplarID, Key, Value, Type)`,
	-- Parent entity lookups (for hierarchical queries - e.g., all attributes for a span including events/links)
	`CREATE INDEX IF NOT EXISTS idx_attributes_span_hierarchy ON attributes(SpanID, EventID, LinkID)`,
	`CREATE INDEX IF NOT EXISTS idx_attributes_metric_hierarchy ON attributes(MetricID, DataPointID, ExemplarID)`,
	-- Key/value search (covering Type for filtering)
	`CREATE INDEX IF NOT EXISTS idx_attributes_key_value ON attributes(Key, Value, Type)`,
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
		FROM logs WHERE ID = ?
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
            COUNT(*) OVER (PARTITION BY s.TraceID) as span_count,
            COUNT(CASE WHEN s.StatusCode = 'ERROR' THEN 1 END) OVER (PARTITION BY s.TraceID) as error_count,
            COUNT(CASE WHEN s.Attributes['exception.type'] IS NOT NULL THEN 1 END) OVER (PARTITION BY s.TraceID) as exception_count
        FROM spans s
        ORDER BY 
            COALESCE(
                MIN(CASE WHEN s.ParentSpanID = '' THEN s.StartTime END) OVER (PARTITION BY s.TraceID),
                MIN(s.StartTime) OVER (PARTITION BY s.TraceID)
            ) DESC,
            s.TraceID,
			CASE WHEN s.ParentSpanID = '' THEN 0 ELSE 1 END
    `

	// SearchTraces is the V2 query template for CTE-based parameter handling
	// The WHERE clause is dynamically added by BuildSQL
	SearchTraces = `
        SELECT DISTINCT ON (s.TraceID)
            s.TraceID,
            CASE WHEN s.ParentSpanID = '' THEN CAST(s.ResourceAttributes['service.name'] AS VARCHAR) END as service_name,
            CASE WHEN s.ParentSpanID = '' THEN s.Name END as root_name,
            CASE WHEN s.ParentSpanID = '' THEN s.StartTime END as root_start_time,
            CASE WHEN s.ParentSpanID = '' THEN s.EndTime END as root_end_time,
            COUNT(*) OVER (PARTITION BY s.TraceID) as span_count,
            COUNT(CASE WHEN s.StatusCode = 'ERROR' THEN 1 END) OVER (PARTITION BY s.TraceID) as error_count,
            COUNT(CASE WHEN s.Attributes['exception.type'] IS NOT NULL THEN 1 END) OVER (PARTITION BY s.TraceID) as exception_count
        FROM spans s
        ORDER BY 
            COALESCE(
                MIN(CASE WHEN s.ParentSpanID = '' THEN s.StartTime END) OVER (PARTITION BY s.TraceID),
                MIN(s.StartTime) OVER (PARTITION BY s.TraceID)
            ) DESC,
            s.TraceID,
			CASE WHEN s.ParentSpanID = '' THEN 0 ELSE 1 END
    `

	// SelectTrace retrieves spans in hierarchical order with depth information
	// This mimics a tree structure that can be easily converted to a tree on the frontend
	SelectTrace = `
		WITH RECURSIVE
		-- Define the trace parameter
		param(traceID) AS (
			VALUES (?)
		),
		-- Get all spans in depth-first order
		spans_tree AS (
			-- Anchor: Start with root spans first, then orphan spans
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
				0 as depth,
				ARRAY[ROW_NUMBER() OVER (ORDER BY 
					CASE WHEN s.ParentSpanID IS NULL OR s.ParentSpanID = '' THEN 0 ELSE 1 END,
					s.StartTime
				)] as sort_path
			FROM spans s, param p
			WHERE s.TraceID = p.traceID 
				AND s.ParentSpanID NOT IN (SELECT SpanID FROM spans WHERE TraceID = p.traceID)
			
			UNION ALL
			
			-- Recursive: Find children of the current span, sorted by StartTime
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
				st.depth + 1,
				st.sort_path || ARRAY[ROW_NUMBER() OVER (
					PARTITION BY st.SpanID 
					ORDER BY s.StartTime
				)] as sort_path
			FROM spans s, param p
			JOIN spans_tree st ON s.ParentSpanID = st.SpanID AND s.TraceID = st.TraceID
			WHERE s.TraceID = p.traceID
		)
		-- Return all spans in depth-first order
		SELECT 
			TraceID, TraceState, SpanID, ParentSpanID, Name, Kind, 
			StartTime, EndTime, Attributes, Events, Links, 
			ResourceAttributes, ResourceDroppedAttributesCount,
			ScopeName, ScopeVersion, ScopeAttributes, ScopeDroppedAttributesCount,
			DroppedAttributesCount, DroppedEventsCount, DroppedLinksCount,
			StatusCode, StatusMessage, depth
		FROM spans_tree
		ORDER BY sort_path
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
		FROM metrics WHERE ID = ?
	`
)

// Maintenance queries
const (
	TruncateSpans   = `TRUNCATE TABLE spans`
	TruncateLogs    = `TRUNCATE TABLE logs`
	TruncateMetrics = `TRUNCATE TABLE metrics`
)

// Targeted deletion queries
const (
	DeleteSpansByTraceID = `DELETE FROM spans WHERE TraceID = ?`
	DeleteSpanByID       = `DELETE FROM spans WHERE SpanID = ?`
	DeleteLogByID        = `DELETE FROM logs WHERE ID = ?`
	DeleteMetricByID     = `DELETE FROM metrics WHERE ID = ?`
)

// Batch deletion queries using IN clause
const (
	DeleteSpansByTraceIDs = `DELETE FROM spans WHERE TraceID IN (%s)`
	DeleteSpansByIDs      = `DELETE FROM spans WHERE SpanID IN (%s)`
	DeleteLogsByIDs       = `DELETE FROM logs WHERE ID IN (%s)`
	DeleteMetricsByIDs    = `DELETE FROM metrics WHERE ID IN (%s)`
)

// Sample data detection and deletion queries
const (
	CheckSampleDataExists = `
		SELECT COUNT(*) FROM spans WHERE ResourceAttributes['telemetry.sample'] = true
	`
	ClearSampleData = `
		DELETE FROM spans WHERE ResourceAttributes['telemetry.sample'] = true;
		DELETE FROM logs WHERE ResourceAttributes['telemetry.sample'] = true;
		DELETE FROM metrics WHERE ResourceAttributes['telemetry.sample'] = true;
	`
)

// Attribute discovery queries
const (
	// GetTraceAttributes discovers all attributes in trace data
	GetTraceAttributes = `
		WITH trace_attributes AS (
			SELECT ResourceAttributes, Attributes, ScopeAttributes, Events, Links
			FROM spans
			WHERE StartTime >= ? AND StartTime <= ?
		),
		events_unnested AS (
			SELECT 
				unnest(Events) AS event_data
			FROM trace_attributes
			WHERE Events IS NOT NULL
		),
		links_unnested AS (
			SELECT 
				unnest(Links) AS link_data
			FROM trace_attributes
			WHERE Links IS NOT NULL
		),
		all_attributes AS (
			-- Resource attributes
			SELECT 
				unnest.key as attribute_name,
				'resource' as attribute_scope,
				union_tag(unnest.value) as attribute_type
			FROM trace_attributes,
			UNNEST(map_entries(ResourceAttributes))
			WHERE ResourceAttributes IS NOT NULL
			
			UNION ALL
			
			-- Span attributes
			SELECT 
				unnest.key as attribute_name,
				'span' as attribute_scope,
				union_tag(unnest.value) as attribute_type
			FROM trace_attributes,
			UNNEST(map_entries(Attributes))
			WHERE Attributes IS NOT NULL
			
			UNION ALL
			
			-- Scope attributes
			SELECT 
				unnest.key as attribute_name,
				'scope' as attribute_scope,
				union_tag(unnest.value) as attribute_type
			FROM trace_attributes,
			UNNEST(map_entries(ScopeAttributes))
			WHERE ScopeAttributes IS NOT NULL
			
			UNION ALL
			
			-- Event attributes
			SELECT 
				unnest.key as attribute_name,
				'event' as attribute_scope,
				union_tag(unnest.value) as attribute_type
			FROM events_unnested,
			unnest(map_entries(event_data.Attributes))
			WHERE event_data.Attributes IS NOT NULL
			
			UNION ALL
			
			-- Link attributes
			SELECT 
				unnest.key as attribute_name,
				'link' as attribute_scope,
				union_tag(unnest.value) as attribute_type
			FROM links_unnested,
			unnest(map_entries(link_data.Attributes))
			WHERE link_data.Attributes IS NOT NULL
		)
		SELECT DISTINCT 
			attribute_name,
			attribute_scope,
			CASE 
				WHEN attribute_type = 'string_list' THEN 'string[]'
				WHEN attribute_type = 'int64_list' THEN 'int64[]'
				WHEN attribute_type = 'float64_list' THEN 'float64[]'
				WHEN attribute_type = 'boolean_list' THEN 'boolean[]'
				ELSE attribute_type
			END as attribute_type
		FROM all_attributes
		ORDER BY attribute_name, attribute_scope
	`
)

// Helper function to build placeholders for IN clause
func buildPlaceholders(count int) string {
	if count == 0 {
		return ""
	}

	placeholders := make([]string, count)
	for i := range count {
		placeholders[i] = "?"
	}
	return strings.Join(placeholders, ",")
}
