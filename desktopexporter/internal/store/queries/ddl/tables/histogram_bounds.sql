-- The bounds dictionary: one row per distinct explicit-bounds vector.
--
-- Bounds may change within a series, so datapoints reference this dictionary.
-- id is SHA-256 over length-prefixed IEEE-754 bits, truncated to a UUID.
create table if not exists histogram_bounds (
		id uuid primary key,
		bounds double[] not null
	)
