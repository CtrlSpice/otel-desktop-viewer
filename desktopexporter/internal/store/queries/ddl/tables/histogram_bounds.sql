-- The bounds dictionary: one row per distinct explicit-bounds vector.
--
-- Bucket boundaries are a property of the instrument, not the observation, so
-- a histogram reporting every few seconds writes the same ~25-double array on
-- every datapoint -- the same shape of duplication the attribute dictionary
-- removed, and one the storage layer's varchar compression cannot touch,
-- because a double[] is not a varchar.
--
-- A dictionary rather than a column on metric_streams: OTel does not promise
-- bounds are fixed per stream, it is only the overwhelming practice. Hashing
-- handles the exception instead of assuming it away -- a stream whose SDK is
-- reconfigured mid-flight simply references a second row.
--
-- id is content-derived (sha256 over the IEEE-754 bits of each bound,
-- length-prefixed, truncated to a uuid), so the same vector hashes to the same
-- row across batches and restarts with no read-back, exactly as attribute ids
-- do.
create table if not exists histogram_bounds (
		id uuid primary key,
		bounds double[] not null
	)
