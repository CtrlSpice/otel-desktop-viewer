-- Folds a new batch of sample pairs into an ingest_rejections row: new pairs
-- first, pairs already present dropped, trimmed to the bound. Newest-first,
-- deduped, bounded -- the shape that keeps a replay loop from growing the row.
create or replace macro merge_samples(new_samples, old_samples) as (
		(new_samples || list_filter(old_samples,
			lambda x: not list_contains(new_samples, x)))[1:10]
	)
