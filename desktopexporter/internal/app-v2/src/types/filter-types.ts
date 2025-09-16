// Filter types for telemetry signal searching - separate from API data types

export interface AttributeFilter {
  name: string;
  value: string;
  operator: 'equals' | 'contains' | 'startsWith';
}

export interface TimeRange {
  start?: string;
  end?: string;
}

export interface TelemetryFilters {
  search?: string;
  serviceName?: string[];
  timeRange: TimeRange;
  attributes?: AttributeFilter[];
  limit?: number;
  offset?: number;
}

// Signal-specific filter types
export interface TraceFilters extends TelemetryFilters {
  // Trace-specific filters can be added here
}

export interface LogFilters extends TelemetryFilters {
  // Log-specific filters can be added here
  level?: string[];
  source?: string[];
}

export interface MetricFilters extends TelemetryFilters {
  // Metric-specific filters can be added here
  metricName?: string[];
  aggregation?: string[];
}
