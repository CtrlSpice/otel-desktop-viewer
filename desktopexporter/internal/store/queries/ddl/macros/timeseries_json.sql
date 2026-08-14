-- timeseries_json: one series in wire shape.
--
-- Every field is passed in. attributes and datapoints are already-built JSON
-- from aggregates the caller ran, and resource comes from a join -- so this
-- assembles rather than resolves, and stays a pure function of its arguments.
create or replace macro timeseries_json(attrs_key, attributes, resource, datapoints, stats) as (
		json_object(
			'attributesKey', attrs_key,
			'attributes', attributes,
			'resource', resource,
			'datapoints', datapoints,
			'stats', stats
		)
	)
