export type SearchSignal = 'traces' | 'logs' | 'metrics'

// Normalized search-field value types. Received discovery kinds are converted
// at the telemetry boundary before they enter this model.
export type FieldType =
  | 'string'
  | 'int64'
  | 'float64'
  | 'boolean'
  | 'string[]'
  | 'int64[]'
  | 'float64[]'
  | 'boolean[]'
  | 'array'
  | 'bytes'
  | 'empty'
  | 'map'

// The owner kinds an attribute search field can target. Mirrors
// JsonAttributeScope in wire-types.ts, which is what the discovery endpoints
// return; the two must stay in step or a discovered field cannot be turned into
// a search field.
export type AttributeScope =
  | 'resource'
  | 'scope'
  | 'span'
  | 'event'
  | 'link'
  | 'log'
  | 'datapoint'
  | 'exemplar'
  | 'metadata'

// Search operators with labels and symbols
export const OPERATORS = {
  EQUALS: { label: 'equals', symbol: '=' },
  NOT_EQUALS: { label: 'does not equal', symbol: '!=' },
  GREATER_THAN: { label: 'greater than', symbol: '>' },
  LESS_THAN: { label: 'less than', symbol: '<' },
  GREATER_THAN_OR_EQUAL: { label: 'greater than or equal', symbol: '>=' },
  LESS_THAN_OR_EQUAL: { label: 'less than or equal', symbol: '<=' },

  // Pattern Matching
  REGEX: { label: 'matches regex', symbol: 'REGEXP' },
  CONTAINS: { label: 'contains', symbol: 'CONTAINS' },
  NOT_CONTAINS: { label: 'does not contain', symbol: 'NOT CONTAINS' },
  LIKE: { label: 'matches pattern', symbol: 'LIKE' },
  NOT_LIKE: { label: 'does not match pattern', symbol: 'NOT LIKE' },

  // Set Operations
  IN: { label: 'is one of', symbol: 'IN' },
  NOT_IN: { label: 'is not one of', symbol: 'NOT IN' },
  // Derived operators: never listed in a field's operator set. IS NULL is
  // legal wherever = is, IS NOT NULL wherever != is, NOT REGEXP wherever
  // REGEXP is. The backend receives an explicit operator, not a sentinel value.
  IS_NULL: { label: 'is null', symbol: 'IS NULL' },
  IS_NOT_NULL: { label: 'is not null', symbol: 'IS NOT NULL' },
  NOT_REGEX: { label: 'does not match regex', symbol: 'NOT REGEXP' },
} as const

export type Operator = (typeof OPERATORS)[keyof typeof OPERATORS]

export type ValueOwner =
  Exclude<AttributeScope, 'metadata'> | 'metric' | 'experiment' | 'result' | ''
export type ValuePath = readonly (string | number)[]

export type FieldReference = {
  owner: ValueOwner
  path: ValuePath
  type: FieldType
}

export type FieldDefinition = FieldReference & {
  name: string
  operators: Operator[]
  description?: string
  enumValues?: readonly string[]
  discoverableValues?: true
}

/** Received paths use the existing selectable-token machinery. */
export function usesFieldToken(field: FieldDefinition): boolean {
  return field.path.length > 1
}

export function fieldIdentity(field: FieldReference): string {
  return JSON.stringify([field.owner, field.path, field.type])
}

/** The existing built-in names remain query aliases for owner-relative paths. */
export function builtinFieldReference(
  name: string,
  owner: ValueOwner
): Pick<FieldReference, 'owner' | 'path'> {
  const dot = name.indexOf('.')
  const prefix = name.slice(0, dot)
  if (
    dot >= 0 &&
    (prefix === 'resource' ||
      prefix === 'scope' ||
      prefix === 'event' ||
      prefix === 'link' ||
      prefix === 'result')
  ) {
    return { owner: prefix, path: [name.slice(dot + 1)] }
  }
  return { owner, path: [name] }
}

// Get appropriate operators based on field type
export function getOperatorsForFieldType(fieldType: FieldType): Operator[] {
  switch (fieldType) {
    case 'string':
      return [
        OPERATORS.EQUALS,
        OPERATORS.NOT_EQUALS,
        OPERATORS.CONTAINS,
        OPERATORS.NOT_CONTAINS,
        OPERATORS.LIKE,
        OPERATORS.NOT_LIKE,
        OPERATORS.REGEX,
        OPERATORS.IN,
        OPERATORS.NOT_IN,
      ]

    case 'int64':
    case 'float64':
      return [
        OPERATORS.EQUALS,
        OPERATORS.NOT_EQUALS,
        OPERATORS.GREATER_THAN,
        OPERATORS.LESS_THAN,
        OPERATORS.GREATER_THAN_OR_EQUAL,
        OPERATORS.LESS_THAN_OR_EQUAL,
        OPERATORS.IN,
        OPERATORS.NOT_IN,
      ]

    case 'boolean':
      return [
        OPERATORS.EQUALS,
        OPERATORS.NOT_EQUALS,
        OPERATORS.IN,
        OPERATORS.NOT_IN,
      ]

    case 'string[]':
    case 'int64[]':
    case 'float64[]':
    case 'boolean[]':
      // Arrays support equality, membership checks, and contains
      return [
        OPERATORS.EQUALS,
        OPERATORS.NOT_EQUALS,
        OPERATORS.CONTAINS,
        OPERATORS.NOT_CONTAINS,
        OPERATORS.IN,
        OPERATORS.NOT_IN,
      ]

    case 'array':
      // Received OTel arrays may be empty or mixed, so only membership is
      // truthful; they have no inferred homogeneous element type.
      return [OPERATORS.CONTAINS, OPERATORS.NOT_CONTAINS]

    default:
      // Fallback to basic operators for unknown types
      return [OPERATORS.EQUALS, OPERATORS.NOT_EQUALS]
  }
}

export type Query = {
  field: FieldDefinition
  operator: Operator
  value: string
}

export type LogicalOperator = 'AND' | 'OR'

export type QueryNode =
  | {
      id: string
      type: 'text'
      query: { operator: Operator; value: string }
    }
  | {
      id: string
      type: 'condition'
      query: Query
    }
  | {
      id: string
      type: 'group'
      group: {
        operator: LogicalOperator
        children: QueryNode[]
      }
    }

/** A parsed search keeps result controls outside the boolean predicate tree. */
export type ParsedSearchRequest = {
  predicate: QueryNode | null
  limit: number | null
}
