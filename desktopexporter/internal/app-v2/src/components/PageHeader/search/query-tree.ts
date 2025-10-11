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

export type QueryNode =
  | {
      id: string;
      type: 'condition';
      condition: Query;
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

// Add a new condition with logical operator
export function addConditionToTree(
  queryTree: QueryNode | null,
  newCondition: Query,
  operator: LogicalOperator
): QueryNode {
  if (!queryTree) {
    return {
      id: generateId(),
      type: 'condition',
      condition: newCondition,
    };
  }

  if (queryTree.type === 'condition') {
    // Convert single condition to group
    return {
      id: generateId(),
      type: 'group',
      group: {
        operator,
        children: [
          queryTree,
          { id: generateId(), type: 'condition', condition: newCondition },
        ],
      },
    };
  } else {
    // Add to existing group
    queryTree.group!.children.push({
      id: generateId(),
      type: 'condition',
      condition: newCondition,
    });
    queryTree.group!.operator = operator;
    return queryTree;
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
export function getAllConditions(queryTree: QueryNode | null): QueryNode[] {
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
