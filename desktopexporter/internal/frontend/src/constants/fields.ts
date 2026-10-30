// Search field definitions for different signal types
import {
  OPERATORS,
  builtinFieldReference,
  fieldIdentity,
  type FieldDefinition,
  type SearchSignal,
  type ValueOwner,
} from '@/search/model'

export type ColumnCategory = 'pinned' | 'flexible' | 'detail'

export type ColumnVisibility = {
  fieldID: string
  label: string
  category: ColumnCategory
}

export const LOG_COLUMN_DEFAULTS: ColumnVisibility[] = [
  { fieldID: 'timestamp', label: 'Timestamp', category: 'pinned' },
  { fieldID: 'service', label: 'Service', category: 'pinned' },
  { fieldID: 'severity', label: 'Severity', category: 'flexible' },
  { fieldID: 'traceID', label: 'Trace ID', category: 'detail' },
  { fieldID: 'body', label: 'Body', category: 'flexible' },
]

export const TRACE_COLUMN_DEFAULTS: ColumnVisibility[] = [
  { fieldID: 'traceID', label: 'Trace ID', category: 'pinned' },
  { fieldID: 'rootName', label: 'Root Name', category: 'pinned' },
  { fieldID: 'service', label: 'Service', category: 'flexible' },
  { fieldID: 'startTime', label: 'Start Time', category: 'flexible' },
  { fieldID: 'duration', label: 'Duration', category: 'flexible' },
  { fieldID: 'spanCount', label: 'Spans', category: 'flexible' },
  { fieldID: 'errorCount', label: 'Errors', category: 'flexible' },
  { fieldID: 'exceptionCount', label: 'Exceptions', category: 'flexible' },
]

type StaticFieldDefinition = Omit<FieldDefinition, 'owner' | 'path'>

function withScalarMembership(
  owner: ValueOwner,
  fields: StaticFieldDefinition[]
): FieldDefinition[] {
  return fields.map(
    ({
      name,
      type,
      operators,
      description,
      enumValues,
      discoverableValues,
    }) => ({
      name,
      type,
      description,
      enumValues,
      discoverableValues,
      ...builtinFieldReference(name, owner),
      operators: [...operators, OPERATORS.IN, OPERATORS.NOT_IN],
    })
  )
}

/** OTLP span kind string names (aligns with pdata). */
export const SPAN_KIND_ENUM = [
  'Unspecified',
  'Internal',
  'Server',
  'Client',
  'Producer',
  'Consumer',
] as const

/** OTLP status code string names (aligns with pdata). */
export const SPAN_STATUS_CODE_ENUM = ['Unset', 'Ok', 'Error'] as const

/** Common OpenTelemetry Logs severity text values. */
export const LOG_SEVERITY_TEXT_ENUM = [
  'TRACE',
  'DEBUG',
  'INFO',
  'WARN',
  'ERROR',
  'FATAL',
] as const

/** Metric instrument / datapoint type strings used in this app’s store and API. */
export const METRIC_TYPE_ENUM = [
  'Gauge',
  'Sum',
  'Histogram',
  'ExponentialHistogram',
  'Empty',
] as const

// Field suggestions based on signal
export function getFieldsBySignal(signal: SearchSignal): FieldDefinition[] {
  const baseFields = [...RESOURCE_FIELDS, ...SCOPE_FIELDS]

  switch (signal) {
    case 'traces':
      return [...baseFields, ...SPAN_FIELDS]

    case 'logs':
      return [...baseFields, ...LOG_FIELDS]

    case 'metrics':
      return [...baseFields, ...METRIC_FIELDS]
  }
}

/** Static searchable fields for CodeMirror suggestions and parsing. */
export function getStaticFieldsForSearch(
  signal: SearchSignal
): FieldDefinition[] {
  return getFieldsBySignal(signal)
}

/** Same searchable field identity (for column filter toggles and detail visibility). */
export function sameFieldDefinition(
  a: FieldDefinition,
  b: FieldDefinition
): boolean {
  return fieldIdentity(a) === fieldIdentity(b)
}

// Span/Trace fields
export const SPAN_FIELDS: FieldDefinition[] = withScalarMembership('span', [
  {
    name: 'traceID',
    type: 'string',
    operators: [OPERATORS.EQUALS, OPERATORS.NOT_EQUALS],
    description: 'Unique identifier for the trace',
  },
  {
    name: 'traceState',
    type: 'string',
    operators: [OPERATORS.EQUALS, OPERATORS.NOT_EQUALS, OPERATORS.CONTAINS],
    description: 'W3C trace state',
  },
  {
    name: 'spanID',
    type: 'string',
    operators: [OPERATORS.EQUALS, OPERATORS.NOT_EQUALS],
    description: 'Unique identifier for the span',
  },
  {
    name: 'parentSpanID',
    type: 'string',
    operators: [OPERATORS.EQUALS, OPERATORS.NOT_EQUALS],
    description: 'ID of the parent span',
  },
  {
    name: 'name',
    discoverableValues: true,
    type: 'string',
    operators: [
      OPERATORS.EQUALS,
      OPERATORS.NOT_EQUALS,
      OPERATORS.CONTAINS,
      OPERATORS.NOT_CONTAINS,
      OPERATORS.LIKE,
      OPERATORS.NOT_LIKE,
      OPERATORS.REGEX,
    ],
    description: 'Name of the span',
  },
  {
    name: 'kind',
    type: 'string',
    operators: [OPERATORS.EQUALS, OPERATORS.NOT_EQUALS],
    description:
      'Span kind (Unspecified, Internal, Server, Client, Producer, Consumer)',
    enumValues: SPAN_KIND_ENUM,
  },
  {
    name: 'kindCode',
    type: 'int64',
    operators: [
      OPERATORS.EQUALS,
      OPERATORS.NOT_EQUALS,
      OPERATORS.GREATER_THAN,
      OPERATORS.LESS_THAN,
    ],
    description: 'Received OTLP span kind number',
  },
  {
    name: 'startTime',
    type: 'int64',
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
    name: 'duration',
    type: 'int64',
    operators: [
      OPERATORS.EQUALS,
      OPERATORS.NOT_EQUALS,
      OPERATORS.GREATER_THAN,
      OPERATORS.LESS_THAN,
      OPERATORS.GREATER_THAN_OR_EQUAL,
      OPERATORS.LESS_THAN_OR_EQUAL,
    ],
    description: 'Span duration in nanoseconds (endTime - startTime)',
  },
  {
    name: 'droppedAttributesCount',
    type: 'int64',
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
    operators: [OPERATORS.EQUALS, OPERATORS.NOT_EQUALS],
    description: 'Status code (Unset, Ok, Error)',
    enumValues: SPAN_STATUS_CODE_ENUM,
  },
  {
    name: 'statusCodeValue',
    type: 'int64',
    operators: [
      OPERATORS.EQUALS,
      OPERATORS.NOT_EQUALS,
      OPERATORS.GREATER_THAN,
      OPERATORS.LESS_THAN,
    ],
    description: 'Received OTLP span status code number',
  },
  {
    name: 'statusMessage',
    type: 'string',
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
    operators: [
      OPERATORS.EQUALS,
      OPERATORS.NOT_EQUALS,
      OPERATORS.CONTAINS,
      OPERATORS.NOT_CONTAINS,
      OPERATORS.LIKE,
      OPERATORS.NOT_LIKE,
      OPERATORS.REGEX,
    ],
    description: 'Name of span events',
  },
  {
    name: 'event.timestamp',
    type: 'int64',
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
    operators: [
      OPERATORS.EQUALS,
      OPERATORS.NOT_EQUALS,
      OPERATORS.GREATER_THAN,
      OPERATORS.LESS_THAN,
    ],
    description: 'Number of event attributes dropped due to limits',
  },
  {
    name: 'flags',
    type: 'int64',
    operators: [OPERATORS.EQUALS, OPERATORS.NOT_EQUALS],
    description:
      'W3C trace flags for the span context (bit 0 sampled, bit 1 remote parent)',
  },
  {
    name: 'link.flags',
    type: 'int64',
    operators: [OPERATORS.EQUALS, OPERATORS.NOT_EQUALS],
    description: 'W3C trace flags for the linked context',
  },
  {
    name: 'link.traceID',
    type: 'string',
    operators: [OPERATORS.EQUALS, OPERATORS.NOT_EQUALS],
    description: 'Trace ID of linked spans',
  },
  {
    name: 'link.spanID',
    type: 'string',
    operators: [OPERATORS.EQUALS, OPERATORS.NOT_EQUALS],
    description: 'Span ID of linked spans',
  },
  {
    name: 'link.traceState',
    type: 'string',
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
    operators: [
      OPERATORS.EQUALS,
      OPERATORS.NOT_EQUALS,
      OPERATORS.GREATER_THAN,
      OPERATORS.LESS_THAN,
    ],
    description: 'Number of link attributes dropped due to limits',
  },
])

// Log-specific fields
export const LOG_FIELDS: FieldDefinition[] = withScalarMembership('log', [
  {
    name: 'timestamp',
    type: 'int64',
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
    operators: [OPERATORS.EQUALS, OPERATORS.NOT_EQUALS],
    description: 'Trace ID associated with this log',
  },
  {
    name: 'spanID',
    type: 'string',
    operators: [OPERATORS.EQUALS, OPERATORS.NOT_EQUALS],
    description: 'Span ID associated with this log',
  },
  {
    name: 'severityText',
    type: 'string',
    operators: [OPERATORS.EQUALS, OPERATORS.NOT_EQUALS],
    description:
      'Text representation of log severity (TRACE, DEBUG, INFO, WARN, ERROR, FATAL)',
    enumValues: LOG_SEVERITY_TEXT_ENUM,
  },
  {
    name: 'severityNumber',
    type: 'int64',
    operators: [
      OPERATORS.EQUALS,
      OPERATORS.NOT_EQUALS,
      OPERATORS.GREATER_THAN,
      OPERATORS.LESS_THAN,
      OPERATORS.GREATER_THAN_OR_EQUAL,
      OPERATORS.LESS_THAN_OR_EQUAL,
    ],
    description: 'Numeric severity level of the log (1-24)',
  },
  {
    name: 'body',
    type: 'string',
    operators: [
      OPERATORS.EQUALS,
      OPERATORS.NOT_EQUALS,
      OPERATORS.CONTAINS,
      OPERATORS.NOT_CONTAINS,
      OPERATORS.LIKE,
      OPERATORS.NOT_LIKE,
      OPERATORS.REGEX,
    ],
    description: 'Log message body/content',
  },
  {
    name: 'droppedAttributesCount',
    type: 'int64',
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
    operators: [OPERATORS.EQUALS, OPERATORS.NOT_EQUALS],
    description: 'Log record flags',
  },
  {
    name: 'eventName',
    discoverableValues: true,
    type: 'string',
    operators: [
      OPERATORS.EQUALS,
      OPERATORS.NOT_EQUALS,
      OPERATORS.CONTAINS,
      OPERATORS.NOT_CONTAINS,
      OPERATORS.LIKE,
      OPERATORS.NOT_LIKE,
      OPERATORS.REGEX,
    ],
    description: 'Name of the event associated with this log',
  },
])

// Metric-specific fields
export const METRIC_FIELDS: FieldDefinition[] = withScalarMembership('metric', [
  {
    name: 'name',
    discoverableValues: true,
    type: 'string',
    operators: [
      OPERATORS.EQUALS,
      OPERATORS.NOT_EQUALS,
      OPERATORS.CONTAINS,
      OPERATORS.NOT_CONTAINS,
      OPERATORS.LIKE,
      OPERATORS.NOT_LIKE,
      OPERATORS.REGEX,
    ],
    description: 'Name of the metric',
  },
  {
    name: 'description',
    type: 'string',
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
    discoverableValues: true,
    type: 'string',
    operators: [
      OPERATORS.EQUALS,
      OPERATORS.NOT_EQUALS,
      OPERATORS.CONTAINS,
      OPERATORS.NOT_CONTAINS,
    ],
    description: 'Unit of measurement for the metric',
  },
  {
    name: 'type',
    type: 'string',
    operators: [OPERATORS.EQUALS, OPERATORS.NOT_EQUALS],
    description:
      'Type of metric (Empty, Gauge, Sum, Histogram, ExponentialHistogram)',
    enumValues: METRIC_TYPE_ENUM,
  },
])

// Instrumentation Scope fields
export const SCOPE_FIELDS: FieldDefinition[] = withScalarMembership('scope', [
  {
    name: 'scope.name',
    type: 'string',
    operators: [
      OPERATORS.EQUALS,
      OPERATORS.NOT_EQUALS,
      OPERATORS.CONTAINS,
      OPERATORS.NOT_CONTAINS,
      OPERATORS.LIKE,
      OPERATORS.NOT_LIKE,
      OPERATORS.REGEX,
    ],
    description: 'Name of the instrumentation scope',
  },
  {
    name: 'scope.version',
    type: 'string',
    operators: [
      OPERATORS.EQUALS,
      OPERATORS.NOT_EQUALS,
      OPERATORS.CONTAINS,
      OPERATORS.NOT_CONTAINS,
      OPERATORS.LIKE,
    ],
    description: 'Version of the instrumentation scope',
  },
  {
    name: 'scope.droppedAttributesCount',
    type: 'int64',
    operators: [
      OPERATORS.EQUALS,
      OPERATORS.NOT_EQUALS,
      OPERATORS.GREATER_THAN,
      OPERATORS.LESS_THAN,
    ],
    description: 'Number of scope attributes dropped due to limits',
  },
])

// Resource fields
export const RESOURCE_FIELDS: FieldDefinition[] = withScalarMembership(
  'resource',
  [
    {
      name: 'serviceIdentity',
      type: 'string',
      operators: [OPERATORS.EQUALS, OPERATORS.NOT_EQUALS],
      description:
        'Catalogue reference for an exact typed service name and namespace',
    },
    {
      name: 'resource.droppedAttributesCount',
      type: 'int64',
      operators: [
        OPERATORS.EQUALS,
        OPERATORS.NOT_EQUALS,
        OPERATORS.GREATER_THAN,
        OPERATORS.LESS_THAN,
      ],
      description: 'Number of resource attributes dropped due to limits',
    },
  ]
)
