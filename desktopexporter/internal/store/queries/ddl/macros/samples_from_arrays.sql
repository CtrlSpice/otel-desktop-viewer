-- Zips two parallel wire-form id arrays into ingest_rejections' sample pairs.
-- Built in SQL from bound arrays so a trace can never be stored beside another
-- occurrence's span.
create or replace macro samples_from_arrays(traces, spans) as (
		list_transform(range(1, len(traces) + 1),
			lambda i: struct_pack("traceID" := traces[i], "spanID" := spans[i]))
	)
