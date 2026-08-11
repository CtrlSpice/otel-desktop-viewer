package queries

// DDL creation order.
//
// These lists are the schema's ordering, and the ordering is load-bearing:
// attributes exists before resources and scopes because they reference it,
// those exist before the signals that carry FKs to them, and macros come last
// because a table macro binds the tables it names at creation time.
//
// The files themselves carry the rationale for each object. Kept as explicit
// lists rather than a directory walk so the order is stated rather than
// implied by filenames, and so a new file that nobody sequenced fails the
// registration check in queries.go instead of silently sorting itself into
// the middle of the schema.
var (
	typeFiles = []string{
		"00_attr_type.sql",
	}

	tableFiles = []string{
		"00_attributes.sql",
		"01_resource_seq.sql",
		"02_scope_seq.sql",
		"03_resources.sql",
		"04_scopes.sql",
		"05_spans.sql",
		"06_events.sql",
		"07_links.sql",
		"08_logs.sql",
		"09_metric_streams.sql",
		"10_metric_series.sql",
		"11_metric_ingests.sql",
		"12_datapoints.sql",
		"13_exemplars.sql",
	}

	indexFiles = []string{
		"00_idx_spans_traceid.sql",
		"01_idx_spans_parentspanid.sql",
		"02_idx_spans_service.sql",
		"03_idx_spans_resource.sql",
		"04_idx_spans_scope.sql",
		"05_idx_events_span.sql",
		"06_idx_links_span.sql",
		"07_idx_links_trace.sql",
		"08_idx_logs_traceid.sql",
		"09_idx_logs_severitynumber.sql",
		"10_idx_logs_service.sql",
		"11_idx_logs_resource.sql",
		"12_idx_logs_scope.sql",
		"13_idx_metric_streams_name.sql",
		"14_idx_metric_streams_service.sql",
		"15_idx_metric_ingests_stream.sql",
		"16_idx_metric_ingests_resource.sql",
		"17_idx_datapoints_stream_time.sql",
		"18_idx_datapoints_series.sql",
		"19_idx_metric_series_stream.sql",
		"20_idx_exemplars_datapoint.sql",
		"21_idx_exemplars_trace.sql",
	}

	macroFiles = []string{
		"00_attr_frame.sql",
		"01_attr_id.sql",
		"02_attrs_json.sql",
		"03_attrs_mapped.sql",
		"04_attrs_key.sql",
		"05_attr_value.sql",
		"06_has_attr.sql",
		"07_trace_id_wire.sql",
		"08_span_id_wire.sql",
		"09_resource_json.sql",
		"10_scope_json.sql",
		"11_attribute_def_json.sql",
		"12_interp_linear.sql",
		"13_interp_loglin.sql",
		"14_hist_buckets.sql",
		"15_exp_pos_buckets.sql",
		"16_exp_neg_buckets.sql",
		"17_exp_zero_bucket.sql",
		"18_exp_buckets.sql",
		"19_bucket_quantile_linear.sql",
		"20_bucket_quantile_loglin.sql",
		"21_hist_quantile.sql",
		"22_exp_hist_quantile.sql",
		"23_floor_div.sql",
		"24_downscale_exp_buckets.sql",
		"25_fold_below_cutoff.sql",
		"26_pad_left_to_offset.sql",
		"27_sum_bucket_vectors.sql",
	}
)
