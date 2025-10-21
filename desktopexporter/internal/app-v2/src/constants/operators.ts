// Search operators with labels and symbols
export const OPERATORS = {
  EQUALS: { label: 'equals', symbols: ['=', ':'] },
  NOT_EQUALS: { label: 'does not equal', symbols: ['!='] },
  CONTAINS: { label: 'contains', symbols: ['~'] },
  DOES_NOT_CONTAIN: { label: 'does not contain', symbols: ['!~'] },
  GREATER_THAN: { label: 'greater than', symbols: ['>'] },
  LESS_THAN: { label: 'less than', symbols: ['<'] },
  GREATER_THAN_OR_EQUAL: { label: 'greater than or equal', symbols: ['>='] },
  LESS_THAN_OR_EQUAL: { label: 'less than or equal', symbols: ['<='] },

  // Pattern Matching
  REGEX: { label: 'matches regex', symbols: ['=~'] },
  STARTS_WITH: { label: 'starts with', symbols: ['^='] },
  ENDS_WITH: { label: 'ends with', symbols: ['$='] },

  // Set Operations
  IN: { label: 'is one of', symbols: ['IN'] },
  NOT_IN: { label: 'is not one of', symbols: ['NOT IN'] },

  // Existence
  EXISTS: { label: 'exists', symbols: ['EXISTS'] },
} as const;

export type Operator = (typeof OPERATORS)[keyof typeof OPERATORS];
