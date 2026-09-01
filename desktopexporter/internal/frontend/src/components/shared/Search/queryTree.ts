import { type FieldDefinition } from '@/constants/fields'
import { type Operator as FieldOperator } from '@/constants/operators'

export type Query = {
  field: FieldDefinition
  operator: FieldOperator
  value: string
}

export type LogicalOperator = 'AND' | 'OR'

export type QueryNode =
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

// Generate unique ID
let nextID = 0
export function generateID(): string {
  return `query-${++nextID}`
}
