-- Aggregate refused telemetry by (signal, kind). Rows have no foreign key
-- because they describe telemetry absent from the store.
create table if not exists ingest_rejections (
		-- sha256(signal, kind) supports index-free upserts.
		id uuid primary key,
		signal varchar not null,
		kind varchar not null,
		-- Bounded, deduplicated (trace ID, span ID) pairs in OTLP hex form.
		samples struct("traceID" varchar, "spanID" varchar)[] not null default [],
		first_seen bigint not null,
		last_seen bigint not null,
		occurrences ubigint not null default 1
	)
