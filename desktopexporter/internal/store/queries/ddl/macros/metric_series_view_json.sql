-- metric_series_view_json: one series in wire shape.
create or replace macro metric_series_view_json(attrs_key, attributes, resource, datapoints, stats, datapoint_count, last_seen_ns, rate_stats, views, sparkline) as (
		json_object(
			'seriesRef', attrs_key,
			'attributes', attributes,
			'resource', resource,
			'datapoints', datapoints,
			'stats', stats,
			-- Full-window count and latest timestamp, before response reduction.
			'datapointCount', datapoint_count,
			'lastSeenNs', last_seen_ns,
			-- Drawn-rate extrema; NULL for histograms or absent rates.
			'rateStats', rate_stats,
			-- Per-bucket Sum, Average, and Rate; NULL for histograms.
			'views', views,
			-- Min/max sparkline at list-row resolution; NULL for histograms.
			'sparkline', sparkline
		)
	)
