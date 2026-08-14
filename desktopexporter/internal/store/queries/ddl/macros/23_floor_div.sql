-- floor_div: mathematical floor division that rounds toward negative
-- infinity. SQL's `/` (and DuckDB's integer divide) truncate toward zero,
-- which is wrong for downscaling exponential histograms with negative
-- bucket indices: e.g. floor(-3 / 2) = -2 (correct, bucket -3 belongs to
-- merged group -2), whereas trunc(-3 / 2) = -1 (wrong group).
--
-- Cast through double to handle bigint inputs without integer-overflow
-- surprises at the boundaries; the floor result is then cast back to
-- bigint so callers can use it as an array index / offset.
create or replace macro floor_div(a, b) as (
		cast(floor(cast(a as double) / cast(b as double)) as bigint)
	)
