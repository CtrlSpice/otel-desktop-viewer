// Search operators with labels and symbols
export const OPERATORS = {
  EQUALS: { label: 'equals', symbol: '=' },
  NOT_EQUALS: { label: 'does not equal', symbol: '!=' },
  LIKE: { label: 'contains', symbol: 'LIKE' },
  NOT_LIKE: { label: 'does not contain', symbol: 'NOT LIKE' },
  GREATER_THAN: { label: 'greater than', symbol: '>' },
  LESS_THAN: { label: 'less than', symbol: '<' },
  GREATER_THAN_OR_EQUAL: { label: 'greater than or equal', symbol: '>=' },
  LESS_THAN_OR_EQUAL: { label: 'less than or equal', symbol: '<=' },

  // Pattern Matching
  REGEX: { label: 'matches regex', symbol: 'REGEXP' },
  STARTS_WITH: { label: 'starts with', symbol: '^' },
  ENDS_WITH: { label: 'ends with', symbol: '$' },

  // Set Operations
  IN: { label: 'is one of', symbol: 'IN' },
  NOT_IN: { label: 'is not one of', symbol: 'NOT IN' },
} as const;

export type Operator = (typeof OPERATORS)[keyof typeof OPERATORS];
