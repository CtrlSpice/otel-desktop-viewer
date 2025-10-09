// Search field definitions for different signal types
import { OPERATORS, type Operator } from './operators';

export interface FieldDefinition {
  name: string;
  searchScope: 'field' | 'attribute';
  operators: Operator[];
  description: string;
}

// Field suggestions based on signal
export function getFieldsBySignal(
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
      // Placeholder for now
      if (view === 'detail') {
        return METRIC_FIELDS;
      }
      return METRIC_FIELDS;

    default:
      return [];
  }
}

// Trace fields
export const SPAN_FIELDS: FieldDefinition[] = [
  {
    name: 'traceId',
    searchScope: 'field',
    operators: [OPERATORS.EQUALS, OPERATORS.NOT_EQUALS],
    description: 'Unique identifier for the trace',
  },
  {
    name: 'spanId',
    searchScope: 'field',
    operators: [OPERATORS.EQUALS, OPERATORS.NOT_EQUALS],
    description: 'Unique identifier for the span',
  },
  {
    name: 'parentSpanId',
    searchScope: 'field',
    operators: [OPERATORS.EQUALS, OPERATORS.NOT_EQUALS],
    description: 'ID of the parent span',
  },
  {
    name: 'name',
    searchScope: 'field',
    operators: [OPERATORS.EQUALS, OPERATORS.NOT_EQUALS, OPERATORS.CONTAINS],
    description: 'Name of the span',
  },
  {
    name: 'kind',
    searchScope: 'field',
    operators: [OPERATORS.EQUALS, OPERATORS.NOT_EQUALS],
    description: 'Span kind (CLIENT, SERVER, etc.)',
  },
  {
    name: 'event.name',
    searchScope: 'field',
    operators: [OPERATORS.EQUALS, OPERATORS.NOT_EQUALS, OPERATORS.CONTAINS],
    description: 'Name of span events',
  },
  {
    name: 'link.traceID',
    searchScope: 'field',
    operators: [OPERATORS.EQUALS, OPERATORS.NOT_EQUALS],
    description: 'Trace ID of linked spans',
  },
  {
    name: 'link.spanID',
    searchScope: 'field',
    operators: [OPERATORS.EQUALS, OPERATORS.NOT_EQUALS],
    description: 'Span ID of linked spans',
  },
  {
    name: 'statusCode',
    searchScope: 'field',
    operators: [OPERATORS.EQUALS, OPERATORS.NOT_EQUALS],
    description: 'Numeric status code of the span',
  },
  {
    name: 'statusMessage',
    searchScope: 'field',
    operators: [OPERATORS.EQUALS, OPERATORS.NOT_EQUALS, OPERATORS.CONTAINS],
    description: 'Human-readable status message',
  },
];

export const TRACE_SUMMARY_FIELDS: FieldDefinition[] = [
  {
    name: 'root.duration',
    searchScope: 'field',
    operators: [
      OPERATORS.EQUALS,
      OPERATORS.NOT_EQUALS,
      OPERATORS.GREATER_THAN,
      OPERATORS.LESS_THAN,
      OPERATORS.GREATER_THAN_OR_EQUAL,
      OPERATORS.LESS_THAN_OR_EQUAL,
    ],
    description: 'Duration of the root span in nanoseconds',
  },
  {
    name: 'root.name',
    searchScope: 'field',
    operators: [OPERATORS.EQUALS, OPERATORS.NOT_EQUALS, OPERATORS.CONTAINS],
    description: 'Name of the root span',
  },
  {
    name: 'root.service.name',
    searchScope: 'field',
    operators: [OPERATORS.EQUALS, OPERATORS.NOT_EQUALS, OPERATORS.CONTAINS],
    description: 'Service name of the root span',
  },
  {
    name: 'spanCount',
    searchScope: 'field',
    operators: [
      OPERATORS.EQUALS,
      OPERATORS.NOT_EQUALS,
      OPERATORS.GREATER_THAN,
      OPERATORS.LESS_THAN,
      OPERATORS.GREATER_THAN_OR_EQUAL,
      OPERATORS.LESS_THAN_OR_EQUAL,
    ],
    description: 'Total number of spans in the trace',
  },
  {
    name: 'errorCount',
    searchScope: 'field',
    operators: [
      OPERATORS.EQUALS,
      OPERATORS.NOT_EQUALS,
      OPERATORS.GREATER_THAN,
      OPERATORS.LESS_THAN,
      OPERATORS.GREATER_THAN_OR_EQUAL,
      OPERATORS.LESS_THAN_OR_EQUAL,
    ],
    description: 'Number of error spans in the trace',
  },
  {
    name: 'exceptionCount',
    searchScope: 'field',
    operators: [
      OPERATORS.EQUALS,
      OPERATORS.NOT_EQUALS,
      OPERATORS.GREATER_THAN,
      OPERATORS.LESS_THAN,
      OPERATORS.GREATER_THAN_OR_EQUAL,
      OPERATORS.LESS_THAN_OR_EQUAL,
    ],
    description: 'Number of exception spans in the trace',
  },
];

// Log-specific fields
export const LOG_FIELDS: FieldDefinition[] = [
  {
    name: 'body',
    searchScope: 'field',
    operators: [OPERATORS.EQUALS, OPERATORS.NOT_EQUALS, OPERATORS.CONTAINS],
    description: 'Log message body/content',
  },
  {
    name: 'severityText',
    searchScope: 'field',
    operators: [OPERATORS.EQUALS, OPERATORS.NOT_EQUALS],
    description: 'Text representation of log severity',
  },
  {
    name: 'severityNumber',
    searchScope: 'field',
    operators: [
      OPERATORS.EQUALS,
      OPERATORS.NOT_EQUALS,
      OPERATORS.GREATER_THAN,
      OPERATORS.LESS_THAN,
      OPERATORS.GREATER_THAN_OR_EQUAL,
      OPERATORS.LESS_THAN_OR_EQUAL,
    ],
    description: 'Numeric severity level of the log',
  },
];

// Metric-specific fields
export const METRIC_FIELDS: FieldDefinition[] = [
  {
    name: 'name',
    searchScope: 'field',
    operators: [OPERATORS.EQUALS, OPERATORS.NOT_EQUALS, OPERATORS.CONTAINS],
    description: 'Name of the metric',
  },
  {
    name: 'description',
    searchScope: 'field',
    operators: [OPERATORS.EQUALS, OPERATORS.NOT_EQUALS, OPERATORS.CONTAINS],
    description: 'Description of what the metric measures',
  },
  {
    name: 'unit',
    searchScope: 'field',
    operators: [OPERATORS.EQUALS, OPERATORS.NOT_EQUALS],
    description: 'Unit of measurement for the metric',
  },
  {
    name: 'type',
    searchScope: 'field',
    operators: [OPERATORS.EQUALS, OPERATORS.NOT_EQUALS],
    description: 'Type of metric (counter, gauge, histogram, etc.)',
  },
];
