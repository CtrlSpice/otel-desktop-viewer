// Search field and attribute suggestions based on signal and view

export interface FieldDefinition {
  name: string;
  searchScope: 'field' | 'attribute' | 'text';
  operators: ('=' | '!=' | '~' | '>' | '<' | '>=' | '<=')[];
  suggestion: string;
  description: string;
}

// Trace fields
export const SPAN_FIELDS: FieldDefinition[] = [
  {
    name: 'traceId',
    searchScope: 'field',
    operators: ['=', '!='],
    suggestion: 'traceId',
    description: 'Unique identifier for the trace',
  },
  {
    name: 'spanId',
    searchScope: 'field',
    operators: ['=', '!='],
    suggestion: 'spanId',
    description: 'Unique identifier for the span',
  },
  {
    name: 'parentSpanId',
    searchScope: 'field',
    operators: ['=', '!='],
    suggestion: 'parentSpanId',
    description: 'ID of the parent span',
  },
  {
    name: 'name',
    searchScope: 'field',
    operators: ['=', '!=', '~'],
    suggestion: 'name',
    description: 'Name of the span',
  },
  {
    name: 'kind',
    searchScope: 'field',
    operators: ['=', '!='],
    suggestion: 'kind',
    description: 'Span kind (CLIENT, SERVER, etc.)',
  },
  {
    name: 'event.name',
    searchScope: 'field',
    operators: ['=', '!=', '~'],
    suggestion: 'event.name',
    description: 'Name of span events',
  },
  {
    name: 'link.traceID',
    searchScope: 'field',
    operators: ['=', '!='],
    suggestion: 'link.traceID',
    description: 'Trace ID of linked spans',
  },
  {
    name: 'link.spanID',
    searchScope: 'field',
    operators: ['=', '!='],
    suggestion: 'link.spanID',
    description: 'Span ID of linked spans',
  },
  {
    name: 'statusCode',
    searchScope: 'field',
    operators: ['=', '!='],
    suggestion: 'statusCode',
    description: 'Numeric status code of the span',
  },
  {
    name: 'statusMessage',
    searchScope: 'field',
    operators: ['=', '!=', '~'],
    suggestion: 'statusMessage',
    description: 'Human-readable status message',
  },
];

export const TRACE_SUMMARY_FIELDS: FieldDefinition[] = [
  {
    name: 'root.duration',
    searchScope: 'field',
    operators: ['=', '!=', '>', '<', '>=', '<='],
    suggestion: 'root.duration',
    description: 'Duration of the root span in nanoseconds',
  },
  {
    name: 'root.name',
    searchScope: 'field',
    operators: ['=', '!=', '~'],
    suggestion: 'root.name',
    description: 'Name of the root span',
  },
  {
    name: 'root.service.name',
    searchScope: 'field',
    operators: ['=', '!=', '~'],
    suggestion: 'root.service.name',
    description: 'Service name of the root span',
  },
  {
    name: 'spanCount',
    searchScope: 'field',
    operators: ['=', '!=', '>', '<', '>=', '<='],
    suggestion: 'spanCount',
    description: 'Total number of spans in the trace',
  },
  {
    name: 'errorCount',
    searchScope: 'field',
    operators: ['=', '!=', '>', '<', '>=', '<='],
    suggestion: 'errorCount',
    description: 'Number of error spans in the trace',
  },
  {
    name: 'exceptionCount',
    searchScope: 'field',
    operators: ['=', '!=', '>', '<', '>=', '<='],
    suggestion: 'exceptionCount',
    description: 'Number of exception spans in the trace',
  },
];

// Log-specific fields
export const LOG_FIELDS: FieldDefinition[] = [
  {
    name: 'body',
    searchScope: 'field',
    operators: ['=', '!=', '~'],
    suggestion: 'body',
    description: 'Log message body/content',
  },
  {
    name: 'severityText',
    searchScope: 'field',
    operators: ['=', '!='],
    suggestion: 'severityText',
    description: 'Text representation of log severity',
  },
  {
    name: 'severityNumber',
    searchScope: 'field',
    operators: ['=', '!=', '>', '<', '>=', '<='],
    suggestion: 'severityNumber',
    description: 'Numeric severity level of the log',
  },
];

// Metric-specific fields
export const METRIC_FIELDS: FieldDefinition[] = [
  {
    name: 'name',
    searchScope: 'field',
    operators: ['=', '!=', '~'],
    suggestion: 'name',
    description: 'Name of the metric',
  },
  {
    name: 'description',
    searchScope: 'field',
    operators: ['=', '!=', '~'],
    suggestion: 'description',
    description: 'Description of what the metric measures',
  },
  {
    name: 'unit',
    searchScope: 'field',
    operators: ['=', '!='],
    suggestion: 'unit',
    description: 'Unit of measurement for the metric',
  },
  {
    name: 'type',
    searchScope: 'field',
    operators: ['=', '!='],
    suggestion: 'type',
    description: 'Type of metric (counter, gauge, histogram, etc.)',
  },
];

// Field suggestions based on signal
export function getFieldSuggestions(
  signal: 'traces' | 'logs' | 'metrics',
  view: 'list' | 'detail'
): FieldDefinition[] {
  switch (signal) {
    case 'traces':
      if (view === 'detail') {
        return SPAN_FIELDS;
      }
      return [...SPAN_FIELDS, ...TRACE_SUMMARY_FIELDS];

    case 'logs':
      // Logs only have list view (no detail view)
      return LOG_FIELDS;

    case 'metrics':
      if (view === 'detail') {
        return METRIC_FIELDS;
      }
      return METRIC_FIELDS; // Same fields for both views

    default:
      return [];
  }
}
