// Search field definitions for different signal types
import { telemetryAPI } from '@/services/telemetry-service';
import { OPERATORS, type Operator } from './operators';

// OpenTelemetry attribute value types
export type FieldType =
  | 'string'
  | 'int64'
  | 'float64'
  | 'boolean'
  | 'string[]'
  | 'int64[]'
  | 'float64[]'
  | 'boolean[]';

export type AttributeScope = 'resource' | 'scope' | 'span' | 'event' | 'link';

export type FieldDefinition =
  | {
      name: string;
      type: FieldType;
      searchScope: 'field';
      operators: Operator[];
      description: string;
    }
  | {
      name: string;
      type: FieldType;
      searchScope: 'attribute';
      attributeScope: AttributeScope;
      operators: Operator[];
      description?: string;
    }
  | {
      name: string;
      type: FieldType;
      searchScope: 'global';
      operators: Operator[];
      description?: string;
    };

// Field suggestions based on signal
export function getFieldsBySignal(
  signal: 'traces' | 'logs' | 'metrics'
): FieldDefinition[] {
  const baseFields = [...RESOURCE_FIELDS, ...SCOPE_FIELDS];

  switch (signal) {
    case 'traces':
      return [...baseFields, ...SPAN_FIELDS];

    case 'logs':
      return [...baseFields, ...LOG_FIELDS];

    case 'metrics':
      return [...baseFields, ...METRIC_FIELDS];
  }
}

// Function to get dynamic attributes
export async function getDynamicAttributes(
  startTime: number,
  endTime: number,
  signal: 'traces' | 'logs' | 'metrics'
): Promise<FieldDefinition[]> {
  switch (signal) {
    case 'traces':
      try {
        const attributes = await telemetryAPI.getTraceAttributes(
          startTime,
          endTime
        );
        return attributes;
      } catch (error) {
        console.warn('Failed to load dynamic attributes:', error);
        return [];
      }

    case 'logs':
      console.log('Not implemented yet');
      return [];

    case 'metrics':
      console.log('Not implemented yet');
      return [];
    default:
      console.log('Unknown signal type: ', signal);
      return [];
  }
}

// Span/Trace fields
export const SPAN_FIELDS: FieldDefinition[] = [
  {
    name: 'traceID',
    type: 'string',
    searchScope: 'field',
    operators: [OPERATORS.EQUALS, OPERATORS.NOT_EQUALS],
    description: 'Unique identifier for the trace',
  },
  {
    name: 'traceState',
    type: 'string',
    searchScope: 'field',
    operators: [OPERATORS.EQUALS, OPERATORS.NOT_EQUALS, OPERATORS.CONTAINS],
    description: 'W3C trace state',
  },
  {
    name: 'spanID',
    type: 'string',
    searchScope: 'field',
    operators: [OPERATORS.EQUALS, OPERATORS.NOT_EQUALS],
    description: 'Unique identifier for the span',
  },
  {
    name: 'parentSpanID',
    type: 'string',
    searchScope: 'field',
    operators: [OPERATORS.EQUALS, OPERATORS.NOT_EQUALS],
    description: 'ID of the parent span',
  },
  {
    name: 'name',
    type: 'string',
    searchScope: 'field',
    operators: [
      OPERATORS.EQUALS,
      OPERATORS.NOT_EQUALS,
      OPERATORS.CONTAINS,
      OPERATORS.NOT_CONTAINS,
      OPERATORS.STARTS_WITH,
      OPERATORS.ENDS_WITH,
      OPERATORS.REGEX,
    ],
    description: 'Name of the span',
  },
  {
    name: 'kind',
    type: 'string',
    searchScope: 'field',
    operators: [
      OPERATORS.EQUALS,
      OPERATORS.NOT_EQUALS,
      OPERATORS.IN,
      OPERATORS.NOT_IN,
    ],
    description:
      'Span kind (Unspecified, Internal, Server, Client, Producer, Consumer)',
  },
  {
    name: 'startTime',
    type: 'int64',
    searchScope: 'field',
    operators: [
      OPERATORS.EQUALS,
      OPERATORS.NOT_EQUALS,
      OPERATORS.GREATER_THAN,
      OPERATORS.LESS_THAN,
      OPERATORS.GREATER_THAN_OR_EQUAL,
      OPERATORS.LESS_THAN_OR_EQUAL,
    ],
    description: 'Start timestamp in nanoseconds',
  },
  {
    name: 'endTime',
    type: 'int64',
    searchScope: 'field',
    operators: [
      OPERATORS.EQUALS,
      OPERATORS.NOT_EQUALS,
      OPERATORS.GREATER_THAN,
      OPERATORS.LESS_THAN,
      OPERATORS.GREATER_THAN_OR_EQUAL,
      OPERATORS.LESS_THAN_OR_EQUAL,
    ],
    description: 'End timestamp in nanoseconds',
  },
  {
    name: 'droppedAttributesCount',
    type: 'int64',
    searchScope: 'field',
    operators: [
      OPERATORS.EQUALS,
      OPERATORS.NOT_EQUALS,
      OPERATORS.GREATER_THAN,
      OPERATORS.LESS_THAN,
    ],
    description: 'Number of attributes dropped due to limits',
  },
  {
    name: 'droppedEventsCount',
    type: 'int64',
    searchScope: 'field',
    operators: [
      OPERATORS.EQUALS,
      OPERATORS.NOT_EQUALS,
      OPERATORS.GREATER_THAN,
      OPERATORS.LESS_THAN,
    ],
    description: 'Number of events dropped due to limits',
  },
  {
    name: 'droppedLinksCount',
    type: 'int64',
    searchScope: 'field',
    operators: [
      OPERATORS.EQUALS,
      OPERATORS.NOT_EQUALS,
      OPERATORS.GREATER_THAN,
      OPERATORS.LESS_THAN,
    ],
    description: 'Number of links dropped due to limits',
  },
  {
    name: 'statusCode',
    type: 'string',
    searchScope: 'field',
    operators: [
      OPERATORS.EQUALS,
      OPERATORS.NOT_EQUALS,
      OPERATORS.IN,
      OPERATORS.NOT_IN,
    ],
    description: 'Status code (Unset, Ok, Error)',
  },
  {
    name: 'statusMessage',
    type: 'string',
    searchScope: 'field',
    operators: [
      OPERATORS.EQUALS,
      OPERATORS.NOT_EQUALS,
      OPERATORS.CONTAINS,
      OPERATORS.NOT_CONTAINS,
      OPERATORS.REGEX,
    ],
    description: 'Human-readable status message',
  },
  {
    name: 'event.name',
    type: 'string',
    searchScope: 'field',
    operators: [
      OPERATORS.EQUALS,
      OPERATORS.NOT_EQUALS,
      OPERATORS.CONTAINS,
      OPERATORS.NOT_CONTAINS,
      OPERATORS.STARTS_WITH,
      OPERATORS.ENDS_WITH,
      OPERATORS.REGEX,
    ],
    description: 'Name of span events',
  },
  {
    name: 'event.timestamp',
    type: 'int64',
    searchScope: 'field',
    operators: [
      OPERATORS.EQUALS,
      OPERATORS.NOT_EQUALS,
      OPERATORS.GREATER_THAN,
      OPERATORS.LESS_THAN,
      OPERATORS.GREATER_THAN_OR_EQUAL,
      OPERATORS.LESS_THAN_OR_EQUAL,
    ],
    description: 'Timestamp of span event in nanoseconds',
  },
  {
    name: 'event.droppedAttributesCount',
    type: 'int64',
    searchScope: 'field',
    operators: [
      OPERATORS.EQUALS,
      OPERATORS.NOT_EQUALS,
      OPERATORS.GREATER_THAN,
      OPERATORS.LESS_THAN,
    ],
    description: 'Number of event attributes dropped due to limits',
  },
  {
    name: 'link.traceID',
    type: 'string',
    searchScope: 'field',
    operators: [OPERATORS.EQUALS, OPERATORS.NOT_EQUALS],
    description: 'Trace ID of linked spans',
  },
  {
    name: 'link.spanID',
    type: 'string',
    searchScope: 'field',
    operators: [OPERATORS.EQUALS, OPERATORS.NOT_EQUALS],
    description: 'Span ID of linked spans',
  },
  {
    name: 'link.traceState',
    type: 'string',
    searchScope: 'field',
    operators: [
      OPERATORS.EQUALS,
      OPERATORS.NOT_EQUALS,
      OPERATORS.CONTAINS,
      OPERATORS.NOT_CONTAINS,
    ],
    description: 'W3C trace state of linked spans',
  },
  {
    name: 'link.droppedAttributesCount',
    type: 'int64',
    searchScope: 'field',
    operators: [
      OPERATORS.EQUALS,
      OPERATORS.NOT_EQUALS,
      OPERATORS.GREATER_THAN,
      OPERATORS.LESS_THAN,
    ],
    description: 'Number of link attributes dropped due to limits',
  },
];

// Log-specific fields
export const LOG_FIELDS: FieldDefinition[] = [
  {
    name: 'timestamp',
    type: 'int64',
    searchScope: 'field',
    operators: [
      OPERATORS.EQUALS,
      OPERATORS.NOT_EQUALS,
      OPERATORS.GREATER_THAN,
      OPERATORS.LESS_THAN,
      OPERATORS.GREATER_THAN_OR_EQUAL,
      OPERATORS.LESS_THAN_OR_EQUAL,
    ],
    description: 'Timestamp when the log was generated in nanoseconds',
  },
  {
    name: 'observedTimestamp',
    type: 'int64',
    searchScope: 'field',
    operators: [
      OPERATORS.EQUALS,
      OPERATORS.NOT_EQUALS,
      OPERATORS.GREATER_THAN,
      OPERATORS.LESS_THAN,
      OPERATORS.GREATER_THAN_OR_EQUAL,
      OPERATORS.LESS_THAN_OR_EQUAL,
    ],
    description: 'Timestamp when the log was observed in nanoseconds',
  },
  {
    name: 'traceID',
    type: 'string',
    searchScope: 'field',
    operators: [OPERATORS.EQUALS, OPERATORS.NOT_EQUALS],
    description: 'Trace ID associated with this log',
  },
  {
    name: 'spanID',
    type: 'string',
    searchScope: 'field',
    operators: [OPERATORS.EQUALS, OPERATORS.NOT_EQUALS],
    description: 'Span ID associated with this log',
  },
  {
    name: 'severityText',
    type: 'string',
    searchScope: 'field',
    operators: [
      OPERATORS.EQUALS,
      OPERATORS.NOT_EQUALS,
      OPERATORS.IN,
      OPERATORS.NOT_IN,
    ],
    description:
      'Text representation of log severity (TRACE, DEBUG, INFO, WARN, ERROR, FATAL)',
  },
  {
    name: 'severityNumber',
    type: 'int64',
    searchScope: 'field',
    operators: [
      OPERATORS.EQUALS,
      OPERATORS.NOT_EQUALS,
      OPERATORS.GREATER_THAN,
      OPERATORS.LESS_THAN,
      OPERATORS.GREATER_THAN_OR_EQUAL,
      OPERATORS.LESS_THAN_OR_EQUAL,
      OPERATORS.IN,
      OPERATORS.NOT_IN,
    ],
    description: 'Numeric severity level of the log (1-24)',
  },
  {
    name: 'body',
    type: 'string',
    searchScope: 'field',
    operators: [
      OPERATORS.EQUALS,
      OPERATORS.NOT_EQUALS,
      OPERATORS.CONTAINS,
      OPERATORS.NOT_CONTAINS,
      OPERATORS.STARTS_WITH,
      OPERATORS.ENDS_WITH,
      OPERATORS.REGEX,
    ],
    description: 'Log message body/content',
  },
  {
    name: 'droppedAttributesCount',
    type: 'int64',
    searchScope: 'field',
    operators: [
      OPERATORS.EQUALS,
      OPERATORS.NOT_EQUALS,
      OPERATORS.GREATER_THAN,
      OPERATORS.LESS_THAN,
    ],
    description: 'Number of attributes dropped due to limits',
  },
  {
    name: 'flags',
    type: 'int64',
    searchScope: 'field',
    operators: [OPERATORS.EQUALS, OPERATORS.NOT_EQUALS],
    description: 'Log record flags',
  },
  {
    name: 'eventName',
    type: 'string',
    searchScope: 'field',
    operators: [
      OPERATORS.EQUALS,
      OPERATORS.NOT_EQUALS,
      OPERATORS.CONTAINS,
      OPERATORS.NOT_CONTAINS,
      OPERATORS.STARTS_WITH,
      OPERATORS.ENDS_WITH,
      OPERATORS.REGEX,
    ],
    description: 'Name of the event associated with this log',
  },
];

// Metric-specific fields
export const METRIC_FIELDS: FieldDefinition[] = [
  {
    name: 'name',
    type: 'string',
    searchScope: 'field',
    operators: [
      OPERATORS.EQUALS,
      OPERATORS.NOT_EQUALS,
      OPERATORS.CONTAINS,
      OPERATORS.NOT_CONTAINS,
      OPERATORS.STARTS_WITH,
      OPERATORS.ENDS_WITH,
      OPERATORS.REGEX,
    ],
    description: 'Name of the metric',
  },
  {
    name: 'description',
    type: 'string',
    searchScope: 'field',
    operators: [
      OPERATORS.EQUALS,
      OPERATORS.NOT_EQUALS,
      OPERATORS.CONTAINS,
      OPERATORS.NOT_CONTAINS,
      OPERATORS.REGEX,
    ],
    description: 'Description of what the metric measures',
  },
  {
    name: 'unit',
    type: 'string',
    searchScope: 'field',
    operators: [
      OPERATORS.EQUALS,
      OPERATORS.NOT_EQUALS,
      OPERATORS.IN,
      OPERATORS.NOT_IN,
      OPERATORS.CONTAINS,
      OPERATORS.NOT_CONTAINS,
    ],
    description: 'Unit of measurement for the metric',
  },
  {
    name: 'type',
    type: 'string',
    searchScope: 'field',
    operators: [
      OPERATORS.EQUALS,
      OPERATORS.NOT_EQUALS,
      OPERATORS.IN,
      OPERATORS.NOT_IN,
    ],
    description:
      'Type of metric (Empty, Gauge, Sum, Histogram, ExponentialHistogram)',
  },
  {
    name: 'received',
    type: 'int64',
    searchScope: 'field',
    operators: [
      OPERATORS.EQUALS,
      OPERATORS.NOT_EQUALS,
      OPERATORS.GREATER_THAN,
      OPERATORS.LESS_THAN,
      OPERATORS.GREATER_THAN_OR_EQUAL,
      OPERATORS.LESS_THAN_OR_EQUAL,
    ],
    description: 'Timestamp when the metric was received in nanoseconds',
  },
];

// Instrumentation Scope fields
export const SCOPE_FIELDS: FieldDefinition[] = [
  {
    name: 'scope.name',
    type: 'string',
    searchScope: 'field',
    operators: [
      OPERATORS.EQUALS,
      OPERATORS.NOT_EQUALS,
      OPERATORS.CONTAINS,
      OPERATORS.NOT_CONTAINS,
      OPERATORS.STARTS_WITH,
      OPERATORS.ENDS_WITH,
      OPERATORS.REGEX,
    ],
    description: 'Name of the instrumentation scope',
  },
  {
    name: 'scope.version',
    type: 'string',
    searchScope: 'field',
    operators: [
      OPERATORS.EQUALS,
      OPERATORS.NOT_EQUALS,
      OPERATORS.CONTAINS,
      OPERATORS.NOT_CONTAINS,
      OPERATORS.STARTS_WITH,
    ],
    description: 'Version of the instrumentation scope',
  },
  {
    name: 'scope.droppedAttributesCount',
    type: 'int64',
    searchScope: 'field',
    operators: [
      OPERATORS.EQUALS,
      OPERATORS.NOT_EQUALS,
      OPERATORS.GREATER_THAN,
      OPERATORS.LESS_THAN,
    ],
    description: 'Number of scope attributes dropped due to limits',
  },
];

// Resource fields
export const RESOURCE_FIELDS: FieldDefinition[] = [
  {
    name: 'resource.droppedAttributesCount',
    type: 'int64',
    searchScope: 'field',
    operators: [
      OPERATORS.EQUALS,
      OPERATORS.NOT_EQUALS,
      OPERATORS.GREATER_THAN,
      OPERATORS.LESS_THAN,
    ],
    description: 'Number of resource attributes dropped due to limits',
  },
];
