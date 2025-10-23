// Search operators with labels and symbols
import type { FieldType } from './fields';

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
  STARTS_WITH: { label: 'starts with', symbol: '^' },
  ENDS_WITH: { label: 'ends with', symbol: '$' },

  // Set Operations
  IN: { label: 'is one of', symbol: 'IN' },
  NOT_IN: { label: 'is not one of', symbol: 'NOT IN' },
} as const;

export type Operator = (typeof OPERATORS)[keyof typeof OPERATORS];

// Get appropriate operators based on field type
export function getOperatorsForFieldType(fieldType: FieldType): Operator[] {
  switch (fieldType) {
    case 'string':
      return [
        OPERATORS.EQUALS,
        OPERATORS.NOT_EQUALS,
        OPERATORS.CONTAINS,
        OPERATORS.NOT_CONTAINS,
        OPERATORS.STARTS_WITH,
        OPERATORS.ENDS_WITH,
        OPERATORS.REGEX,
        OPERATORS.IN,
        OPERATORS.NOT_IN,
      ];

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
      ];

    case 'boolean':
      return [
        OPERATORS.EQUALS,
        OPERATORS.NOT_EQUALS,
        OPERATORS.IN,
        OPERATORS.NOT_IN,
      ];

    case 'string[]':
      return [
        OPERATORS.EQUALS,
        OPERATORS.NOT_EQUALS,
        OPERATORS.CONTAINS,
        OPERATORS.NOT_CONTAINS,
        OPERATORS.IN,
        OPERATORS.NOT_IN,
      ];

    case 'int64[]':
    case 'float64[]':
      return [
        OPERATORS.EQUALS,
        OPERATORS.NOT_EQUALS,
        OPERATORS.GREATER_THAN,
        OPERATORS.LESS_THAN,
        OPERATORS.GREATER_THAN_OR_EQUAL,
        OPERATORS.LESS_THAN_OR_EQUAL,
        OPERATORS.IN,
        OPERATORS.NOT_IN,
      ];

    case 'boolean[]':
      return [
        OPERATORS.EQUALS,
        OPERATORS.NOT_EQUALS,
        OPERATORS.IN,
        OPERATORS.NOT_IN,
      ];

    default:
      // Fallback to basic operators for unknown types
      return [OPERATORS.EQUALS, OPERATORS.NOT_EQUALS];
  }
}
