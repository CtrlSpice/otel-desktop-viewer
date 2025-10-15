import { type FieldDefinition } from '@/constants/fields';
import { type Operator as FieldOperator } from '@/constants/operators';

export type Query = {
  predicate?: {
    field: FieldDefinition;
    operator: FieldOperator;
  };
  value: string;
};

export type LogicalOperator = 'AND' | 'OR';

export const LOGICAL_OPERATORS = [
  { operator: 'AND' as LogicalOperator, label: 'Add AND condition' },
  { operator: 'OR' as LogicalOperator, label: 'Add OR condition' },
] as const;

export type QueryNode =
  | {
      id: string;
      type: 'condition';
      query: Query;
    }
  | {
      id: string;
      type: 'group';
      group: {
        operator: LogicalOperator;
        children: QueryNode[];
      };
    };

// Generate unique ID
let nextId = 0;
export function generateId(): string {
  return `query-${++nextId}`;
}

// Add a new condition with optional logical operator
export function addConditionToTree(
  queryTree: QueryNode | null,
  newCondition: Query,
  operator?: LogicalOperator
): QueryNode {
  // Validate the new condition
  // Note: We can have a condition without a predicate, but we need a value
  if (!newCondition.value.trim()) {
    throw new Error('Condition must have a value');
  }

  // First condition: We create a single condition node (no logical operator)
  if (!queryTree) {
    return {
      id: generateId(),
      type: 'condition',
      query: newCondition,
    };
  }

  // Subsequent condition: We convert the single condition to a group
  // and add the new condition to this group with the logical operator
  if (queryTree.type === 'condition') {
    if (!operator) {
      throw new Error('Use AND/OR to add conditions');
    }
    return {
      id: generateId(),
      type: 'group',
      group: {
        operator,
        children: [
          queryTree,
          { id: generateId(), type: 'condition', query: newCondition },
        ],
      },
    };
  } else {
    // Adding a condition to an existing group
    if (operator && operator !== queryTree.group!.operator) {
      // Different operator: create new group structure
      return {
        id: generateId(),
        type: 'group',
        group: {
          operator,
          children: [
            queryTree, // Wrap existing group
            { id: generateId(), type: 'condition', query: newCondition },
          ],
        },
      };
    } else {
      // Same operator or no operator: add to existing group
      queryTree.group!.children.push({
        id: generateId(),
        type: 'condition',
        query: newCondition,
      });
      return queryTree;
    }
  }
}

// Remove a condition by ID
export function removeConditionFromTree(
  queryTree: QueryNode | null,
  id: string
): QueryNode | null {
  if (!queryTree) return null;

  if (queryTree.type === 'condition' && queryTree.id === id) {
    // Remove the only condition
    return null;
  }

  if (queryTree.type === 'group') {
    // Remove from group
    queryTree.group.children = queryTree.group.children.filter(
      child => child.id !== id
    );

    // Handle group restructuring
    if (queryTree.group.children.length === 0) {
      // Empty group - remove it
      return null;
    } else if (queryTree.group.children.length === 1) {
      // Single child left - promote it to root
      // This removes unnecessary group wrapper when only one condition remains
      // The child becomes the new root, eliminating the group structure
      return queryTree.group.children[0];
    }
  }

  return queryTree;
}

// Get all condition nodes from a tree (for rendering)
export function getAllConditions(
  queryTree: QueryNode | null
): Array<QueryNode & { type: 'condition' }> {
  if (!queryTree) return [];

  if (queryTree.type === 'condition') {
    return [queryTree];
  }

  // Flatten all conditions from group
  return queryTree.group.children.flatMap(child =>
    child.type === 'condition' ? [child] : getAllConditions(child)
  );
}

// Get logical operators between conditions (for rendering)
export function getLogicalOperators(
  queryTree: QueryNode | null
): LogicalOperator[] {
  if (!queryTree || queryTree.type === 'condition') {
    return [];
  }

  const operators: LogicalOperator[] = [];
  const children = queryTree.group!.children;

  for (let i = 0; i < children.length - 1; i++) {
    operators.push(queryTree.group!.operator);
  }

  return operators;
}
