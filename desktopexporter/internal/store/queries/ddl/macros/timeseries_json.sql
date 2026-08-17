-- timeseries_json: one series in wire shape.
--
-- Every field is passed in. attributes and datapoints are already-built JSON
-- from aggregates the caller ran, and resource comes from a join -- so this
-- assembles rather than resolves, and stays a pure function of its arguments.
create or replace macro timeseries_json(attrs_key, attributes, resource, datapoints, stats, datapoint_count, last_seen_ns, rate_stats, views, sparkline) as (
		json_object(
			'attributesKey', attrs_key,
			'attributes', attributes,
			'resource', resource,
			'datapoints', datapoints,
			'stats', stats,
			-- How many datapoints the window holds for this series, and when it
			-- last reported. Both describe the window, not this response: the
			-- datapoints array is narrowed to the series being drawn and reduced
			-- besides, so counting it answers a different question than the one
			-- a legend badge asks.
			'datapointCount', datapoint_count,
			'lastSeenNs', last_seen_ns,
			-- Extremes of the drawn rate line, for the rate view's badges; the
			-- raw stats above describe the values, these describe the transform.
			-- Null for histograms and for series with no rate to draw.
			'rateStats', rate_stats,
			-- Per-bucket Sum / Average / Rate for this series. Null for
			-- histograms, which have no scalar to aggregate.
			'views', views,
			-- This series' shape at list-row resolution: min and max per bucket,
			-- sized for a 128px box. Sent for every series, including the ones the
			-- user has unchecked, because the row sparkline is how they decide what
			-- to check. Null for histograms, as views is.
			'sparkline', sparkline
		)
	)
