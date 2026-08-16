-- timeseries_json: one series in wire shape.
--
-- Every field is passed in. attributes and datapoints are already-built JSON
-- from aggregates the caller ran, and resource comes from a join -- so this
-- assembles rather than resolves, and stays a pure function of its arguments.
create or replace macro timeseries_json(attrs_key, attributes, resource, datapoints, stats, views, sparkline) as (
		json_object(
			'attributesKey', attrs_key,
			'attributes', attributes,
			'resource', resource,
			'datapoints', datapoints,
			'stats', stats,
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
