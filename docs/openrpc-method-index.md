# JSON-RPC method index

Each link points to one method’s reference on the viewer. Send calls to `POST /rpc`. For HTTP routes, see `/openapi.json`.
Use methods marked **UI view** when the question is about what the viewer displays.

| Method | Description |
| --- | --- |
| [`clearLogs`](/openrpc/clearLogs.json) | Delete **all** retained logs. |
| [`clearMetrics`](/openrpc/clearMetrics.json) | Delete **all** retained Metrics. |
| [`clearTraces`](/openrpc/clearTraces.json) | Delete **all** retained traces and their spans. |
| [`deleteLogsByRefs`](/openrpc/deleteLogsByRefs.json) | Delete the logs identified by the supplied `logRef` values. |
| [`deleteMetric`](/openrpc/deleteMetric.json) | Delete one Metric and its dependent records. |
| [`deleteSpansByTraceID`](/openrpc/deleteSpansByTraceID.json) | Delete all spans in the specified traces. |
| [`discoverTelemetry`](/openrpc/discoverTelemetry.json) | Find services, connections, spans behind a connection, and attribute keys and value counts. |
| [`getFieldValueCompletions`](/openrpc/getFieldValueCompletions.json) | Suggest text completions for a search field. |
| [`getImportConfig`](/openrpc/getImportConfig.json) | Find the OTLP HTTP port used for import. |
| [`getLog`](/openrpc/getLog.json) | Read the full typed body and context for one `logRef`. |
| [`getLogAttributeDefinitions`](/openrpc/getLogAttributeDefinitions.json) | List attribute keys, types, and scopes available to log filters. |
| [`getLogErrorNavigation`](/openrpc/getLogErrorNavigation.json) | Find the previous or next error in a log search. |
| [`getMetric`](/openrpc/getMetric.json) | Read a Metric descriptor and find its series references. |
| [`getMetricAggregateView`](/openrpc/getMetricAggregateView.json) | **UI view:** Read the combined chart across Metric series. For exact datapoints, use `getMetricSeries`. |
| [`getMetricAttributeDefinitions`](/openrpc/getMetricAttributeDefinitions.json) | List attribute keys, types, and scopes available to Metric filters. |
| [`getMetricSeries`](/openrpc/getMetricSeries.json) | Read exact datapoints and exemplars for one Metric series. |
| [`getMetricSeriesOrder`](/openrpc/getMetricSeriesOrder.json) | Get series references sorted by an attribute. |
| [`getMetricView`](/openrpc/getMetricView.json) | **UI view:** Read Metric charts, statistics, and sparklines. For exact datapoints, use `getMetricSeries`. |
| [`getServiceCount`](/openrpc/getServiceCount.json) | Count distinct services across the database. |
| [`getServiceLabels`](/openrpc/getServiceLabels.json) | Resolve a service reference to its full name and namespace. |
| [`getSpan`](/openrpc/getSpan.json) | Read a span, its events, links, and matching logs by trace ID and span ID. |
| [`getStats`](/openrpc/getStats.json) | Check retained record counts, storage use, and ingestion rejections. |
| [`getTraceAttributeDefinitions`](/openrpc/getTraceAttributeDefinitions.json) | List attribute keys, types, and scopes available to trace filters. |
| [`getTraceAttributeDefinitionsByTraceID`](/openrpc/getTraceAttributeDefinitionsByTraceID.json) | List filter attributes for one trace. |
| [`getTraceLogSummaries`](/openrpc/getTraceLogSummaries.json) | Find logs carrying a given trace ID. |
| [`getTraceOverview`](/openrpc/getTraceOverview.json) | List a trace’s spans and log previews. |
| [`getTraceSpanCount`](/openrpc/getTraceSpanCount.json) | Count stored spans in one trace. |
| [`getTraceView`](/openrpc/getTraceView.json) | **UI view:** Read the waterfall hierarchy and search highlights. To inspect the trace, use `getTraceOverview` or `getSpan`. |
| [`query`](/openrpc/query.json) | Run read-only DuckDB SQL against stored telemetry. |
| [`rpc.discover`](/openrpc/rpc.discover.json) | Retrieve the full OpenRPC document over JSON-RPC. |
| [`searchAttributeMatches`](/openrpc/searchAttributeMatches.json) | Find attribute keys or sample typed values at a selected path, without time bounds. |
| [`searchLogPage`](/openrpc/searchLogPage.json) | Read a page of matching logs and the total match count. |
| [`searchLogSummaries`](/openrpc/searchLogSummaries.json) | Find matching logs and their `logRef` values. |
| [`searchMetricSummaries`](/openrpc/searchMetricSummaries.json) | Find matching Metrics and their `metricRef` values. |
| [`searchTraceSummaries`](/openrpc/searchTraceSummaries.json) | Find matching traces and their IDs. |
