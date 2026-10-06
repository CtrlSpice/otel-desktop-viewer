-- Element-wise HUGEINT sum, zero-padding unequal vectors and NULL slots.
-- HUGEINT preserves derived totals above uint64. NULL or empty input returns
-- NULL because DuckDB list_reduce rejects an empty list.
create or replace macro sum_bucket_vectors(vectors) as (
		case
			when vectors is null or len(vectors) = 0 then null
			else list_reduce(
				list_transform(vectors,
					lambda v: list_transform(v, lambda value: value::hugeint)),
				lambda acc, v: list_transform(
					list_zip(acc, v),
					lambda pair: coalesce(pair[1], 0) + coalesce(pair[2], 0)
				)
			)
		end
	)
