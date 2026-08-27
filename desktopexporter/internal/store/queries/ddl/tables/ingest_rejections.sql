-- Telemetry the store would not write, so the viewer can say so instead of
-- rendering as though it arrived. The batch itself succeeds -- a refused row
-- is not an error -- so without this row nothing would ever mention it.
--
-- One row per (signal, kind) rather than per refused item. A sender replaying
-- a capture refuses thousands of spans for one reason, and the useful fact is
-- "4,127 spans, since 14:32", not four thousand copies of it. That keying also
-- bounds the table by how many kinds of problem exist rather than by traffic,
-- so it needs no eviction.
--
-- No foreign key to anything: the whole point is describing rows that are not
-- in the store, and the ones that are get pruned by retention independently.
-- Same reasoning as logs and exemplars referencing spans without one.
create table if not exists ingest_rejections (
		-- sha256(signal, kind), so the upsert can find the row without a
		-- secondary index and two senders hitting the same fault share it.
		id uuid primary key,
		signal varchar not null,
		kind varchar not null,
		-- The most recently refused spans, newest first, in OTLP wire form
		-- (dash-less hex), each ready for a /traces/{trace}?span={span} link.
		-- Both halves per entry, because a span id only identifies a span
		-- within its trace. Bounded at write time, deduped by pair, so a
		-- replay loop cannot grow it. The shape is the diagnosis: entries
		-- sharing one trace say replay, one pair repeating says an
		-- instrumentation bug minting a fixed id, varied traces from one
		-- service point at that service's generator.
		samples struct("traceID" varchar, "spanID" varchar)[] not null default [],
		first_seen bigint not null,
		last_seen bigint not null,
		occurrences ubigint not null default 1
	)
