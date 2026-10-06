-- Mathematical floor division for negative bucket indices; DuckDB integer
-- division truncates toward zero. Return BIGINT for array indices and offsets.
create or replace macro floor_div(a, b) as (
		cast(floor(cast(a as double) / cast(b as double)) as bigint)
	)
