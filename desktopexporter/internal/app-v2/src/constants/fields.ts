// Search field definitions for different signal types
import { OPERATORS, type Operator } from './operators';
import { resourceAttributes } from './resourceAttributes';
import { traceAttributes } from './traceAttributes';
import { GENERAL_ATTRIBUTES } from './generalAttributes';

// OpenTelemetry attribute value types
export type FieldType =
  | 'string'
  | 'int64'
  | 'double'
  | 'boolean'
  | 'string[]'
  | 'int64[]'
  | 'double[]'
  | 'boolean[]';

export type FieldDefinition =
  | {
      name: string;
      type: FieldType;
      searchScope: 'field' | 'global';
      operators: Operator[];
      description: string;
    }
  | {
      name: string;
      type: FieldType;
      searchScope: 'attribute';
      attributeScope: 'resource' | 'span' | 'log' | 'metric' | 'general';
      operators: Operator[];
      description: string;
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

// Attribute suggestions based on signal
export function getAttributesBySignal(
  signal: 'traces' | 'logs' | 'metrics'
): FieldDefinition[] {
  const baseAttributes = [
    ...getResourceAttributes(),
    ...getGeneralAttributes(),
  ];

  switch (signal) {
    case 'traces':
      return [...baseAttributes, ...getTraceAttributes()];

    case 'logs':
      return baseAttributes;

    case 'metrics':
      return baseAttributes;
  }
}

// Helper functions to get attributes
function getResourceAttributes(): FieldDefinition[] {
  return Object.values(resourceAttributes).flat();
}

function getTraceAttributes(): FieldDefinition[] {
  return Object.values(traceAttributes).flat();
}

function getGeneralAttributes(): FieldDefinition[] {
  return Object.values(GENERAL_ATTRIBUTES).flat();
}

// Get attribute prefixes grouped by signal for prefix-aware matching
export function getAttributePrefixesBySignal(
  signal: 'traces' | 'logs' | 'metrics'
): Record<string, FieldDefinition[]> {
  const baseAttributeGroups = {
    ...getResourceAttributeGroups(),
    ...getGeneralAttributeGroups(),
  };

  switch (signal) {
    case 'traces':
      return {
        ...baseAttributeGroups,
        ...getTraceAttributeGroups(),
      };
    case 'logs':
      return baseAttributeGroups;
    case 'metrics':
      return baseAttributeGroups;
  }
}

// Helper functions to get grouped attributes
function getResourceAttributeGroups(): Record<string, FieldDefinition[]> {
  return resourceAttributes;
}

function getTraceAttributeGroups(): Record<string, FieldDefinition[]> {
  return traceAttributes;
}

function getGeneralAttributeGroups(): Record<string, FieldDefinition[]> {
  return GENERAL_ATTRIBUTES;
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

// Global search field
export const GLOBAL_FIELD: FieldDefinition = {
  name: '_global',
  type: 'string',
  searchScope: 'global',
  operators: [OPERATORS.CONTAINS, OPERATORS.NOT_CONTAINS],
  description: 'Search all fields and attributes',
};
