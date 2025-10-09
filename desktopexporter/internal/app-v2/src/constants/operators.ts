// Search operators with labels and symbols
export const OPERATORS = {
  EQUALS: { label: 'equals', symbol: '=' },
  NOT_EQUALS: { label: 'does not equal', symbol: '!=' },
  CONTAINS: { label: 'contains', symbol: '~' },
  DOES_NOT_CONTAIN: { label: 'does not contain', symbol: '!~' },
  GREATER_THAN: { label: 'greater than', symbol: '>' },
  LESS_THAN: { label: 'less than', symbol: '<' },
  GREATER_THAN_OR_EQUAL: { label: 'greater than or equal', symbol: '>=' },
  LESS_THAN_OR_EQUAL: { label: 'less than or equal', symbol: '<=' },
} as const;

export type Operator = (typeof OPERATORS)[keyof typeof OPERATORS];
