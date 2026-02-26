package store

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
		StatusMessage VARCHAR,
		SearchText VARCHAR
	)`,
	`CREATE TABLE IF NOT EXISTS events (
		ID UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		SpanID VARCHAR NOT NULL,
		Name VARCHAR,
		Timestamp BIGINT,
		DroppedAttributesCount UINTEGER,
		SearchText VARCHAR,
		FOREIGN KEY (SpanID) REFERENCES spans(SpanID)
	)`,
	`CREATE TABLE IF NOT EXISTS links (
		ID UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		SpanID VARCHAR NOT NULL,
		TraceID VARCHAR,
		LinkedSpanID VARCHAR,
		TraceState VARCHAR,
		DroppedAttributesCount UINTEGER,
		SearchText VARCHAR,
		FOREIGN KEY (SpanID) REFERENCES spans(SpanID)
	)`,
	`CREATE TABLE IF NOT EXISTS logs (
		ID UUID PRIMARY KEY DEFAULT gen_random_uuid(),
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
		EventName VARCHAR,
		SearchText VARCHAR
	)`,
	`CREATE TABLE IF NOT EXISTS metrics (
		ID UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		Name VARCHAR,
		Description VARCHAR,
		Unit VARCHAR,
		ResourceDroppedAttributesCount UINTEGER,
		ScopeName VARCHAR,
		ScopeVersion VARCHAR,
		ScopeDroppedAttributesCount UINTEGER,
		Received BIGINT,
		SearchText VARCHAR
	)`,
	`CREATE TABLE IF NOT EXISTS datapoints (
		ID UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		MetricID UUID NOT NULL,
		MetricType VARCHAR NOT NULL,
		Timestamp BIGINT,
		StartTime BIGINT,
		Flags UINTEGER,
		DoubleValue DOUBLE,
		IntValue BIGINT,
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
		FOREIGN KEY (MetricID) REFERENCES metrics(ID)
	)`,
	`CREATE TABLE IF NOT EXISTS exemplars (
		ID UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		DataPointID UUID NOT NULL,
		Timestamp BIGINT,
		Value DOUBLE,
		TraceID VARCHAR,
		SpanID VARCHAR,
		FOREIGN KEY (DataPointID) REFERENCES datapoints(ID)
	)`,
	`CREATE TABLE IF NOT EXISTS attributes (
		SpanID VARCHAR,
		EventID UUID,
		LinkID UUID,
		LogID UUID,
		MetricID UUID,
		DataPointID UUID,
		ExemplarID UUID,
		Scope VARCHAR NOT NULL,
		-- Attribute data
		Key VARCHAR NOT NULL,
		Value VARCHAR NOT NULL,
		Type attr_type NOT NULL,
		-- Foreign keys
		FOREIGN KEY (SpanID) REFERENCES spans(SpanID),
		FOREIGN KEY (EventID) REFERENCES events(ID),
		FOREIGN KEY (LinkID) REFERENCES links(ID),
		FOREIGN KEY (LogID) REFERENCES logs(ID),
		FOREIGN KEY (MetricID) REFERENCES metrics(ID),
		FOREIGN KEY (DataPointID) REFERENCES datapoints(ID),
		FOREIGN KEY (ExemplarID) REFERENCES exemplars(ID),
		-- Unique constraint: IDs + Scope + Key (same key can exist per scope per entity)
		UNIQUE (SpanID, EventID, LinkID, LogID, MetricID, DataPointID, ExemplarID, Scope, Key)
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
		MetricType IN ('Gauge', 'Sum', 'Histogram', 'ExponentialHistogram', 'Empty')
	)`,
	// Enforce Empty type: all value/distribution columns must be NULL
	`ALTER TABLE datapoints ADD CONSTRAINT chk_empty_fields CHECK (
		(MetricType != 'Empty') OR (
			DoubleValue IS NULL AND IntValue IS NULL AND ValueType IS NULL AND
			IsMonotonic IS NULL AND AggregationTemporality IS NULL AND
			Count IS NULL AND Sum IS NULL AND Min IS NULL AND Max IS NULL AND
			BucketCounts IS NULL AND ExplicitBounds IS NULL AND
			Scale IS NULL AND ZeroCount IS NULL AND
			PositiveBucketOffset IS NULL AND PositiveBucketCounts IS NULL AND
			NegativeBucketOffset IS NULL AND NegativeBucketCounts IS NULL
		)
	)`,
	// Enforce Gauge type: one of DoubleValue/IntValue and ValueType must be NOT NULL, histogram fields must be NULL
	`ALTER TABLE datapoints ADD CONSTRAINT chk_gauge_fields CHECK (
		(MetricType != 'Gauge') OR (
			ValueType IS NOT NULL AND (DoubleValue IS NOT NULL OR IntValue IS NOT NULL) AND
			Count IS NULL AND Sum IS NULL AND Min IS NULL AND Max IS NULL AND
			BucketCounts IS NULL AND ExplicitBounds IS NULL AND
			Scale IS NULL AND ZeroCount IS NULL AND
			PositiveBucketOffset IS NULL AND PositiveBucketCounts IS NULL AND
			NegativeBucketOffset IS NULL AND NegativeBucketCounts IS NULL AND
			AggregationTemporality IS NULL
		)
	)`,
	// Enforce Sum type: one of DoubleValue/IntValue, ValueType, IsMonotonic, AggregationTemporality must be NOT NULL, histogram fields must be NULL
	`ALTER TABLE datapoints ADD CONSTRAINT chk_sum_fields CHECK (
		(MetricType != 'Sum') OR (
			ValueType IS NOT NULL AND (DoubleValue IS NOT NULL OR IntValue IS NOT NULL) AND
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
			DoubleValue IS NULL AND IntValue IS NULL AND ValueType IS NULL AND IsMonotonic IS NULL AND
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
			DoubleValue IS NULL AND IntValue IS NULL AND ValueType IS NULL AND IsMonotonic IS NULL AND
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
	`CREATE INDEX IF NOT EXISTS idx_logs_searchtext ON logs(SearchText)`,
	`CREATE INDEX IF NOT EXISTS idx_metrics_name ON metrics(Name)`,
	`CREATE INDEX IF NOT EXISTS idx_metrics_received ON metrics(Received)`,
	`CREATE INDEX IF NOT EXISTS idx_metrics_searchtext ON metrics(SearchText)`,
	`CREATE INDEX IF NOT EXISTS idx_datapoints_type_metric_time ON datapoints(MetricType, MetricID, Timestamp DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_datapoints_metric_time ON datapoints(MetricID, Timestamp DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_datapoints_time ON datapoints(Timestamp DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_exemplars_datapoint ON exemplars(DataPointID)`,
	`CREATE INDEX IF NOT EXISTS idx_exemplars_trace ON exemplars(TraceID, SpanID)`,
	// Direct entity lookups (covering Key, Value, Type for common queries)
	`CREATE INDEX IF NOT EXISTS idx_attributes_span ON attributes(SpanID, Key, Value, Type)`,
	`CREATE INDEX IF NOT EXISTS idx_attributes_event ON attributes(EventID, Key, Value, Type)`,
	`CREATE INDEX IF NOT EXISTS idx_attributes_link ON attributes(LinkID, Key, Value, Type)`,
	`CREATE INDEX IF NOT EXISTS idx_attributes_log ON attributes(LogID, Key, Value, Type)`,
	`CREATE INDEX IF NOT EXISTS idx_attributes_metric ON attributes(MetricID, Key, Value, Type)`,
	`CREATE INDEX IF NOT EXISTS idx_attributes_datapoint ON attributes(DataPointID, Key, Value, Type)`,
	`CREATE INDEX IF NOT EXISTS idx_attributes_exemplar ON attributes(ExemplarID, Key, Value, Type)`,
	// Parent entity lookups (for hierarchical queries - e.g., all attributes for a span including events/links)
	`CREATE INDEX IF NOT EXISTS idx_attributes_span_hierarchy ON attributes(SpanID, EventID, LinkID)`,
	`CREATE INDEX IF NOT EXISTS idx_attributes_metric_hierarchy ON attributes(MetricID, DataPointID, ExemplarID)`,
	// Key/value search (covering Type for filtering)
	`CREATE INDEX IF NOT EXISTS idx_attributes_key_value ON attributes(Key, Value, Type)`,
}
