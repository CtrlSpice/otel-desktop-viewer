-- Aggregation helper: element-wise sum of a list of equal-length numeric
-- lists. Used to merge bucket_counts arrays across multiple histogram
-- streams that share the same explicit_bounds. The caller is responsible
-- for enforcing the shared-bounds invariant; this macro is intentionally
-- permissive about length mismatches (zero-pads via list_zip + coalesce)
-- so a programmer error there yields slightly-off numbers rather than a
-- crash.
--
-- Returns NULL for NULL or empty input -- DuckDB's list_reduce raises a
-- hard error on an empty list, so we guard explicitly. NULL slots inside
-- an element list are coalesced to 0.
create or replace macro sum_bucket_vectors(vectors) as (
		case
			when vectors is null or len(vectors) = 0 then null
			else list_reduce(
				vectors,
				(acc, v) -> list_transform(
					list_zip(acc, v),
					pair -> coalesce(pair[1], 0) + coalesce(pair[2], 0)
				)
			)
		end
	)
