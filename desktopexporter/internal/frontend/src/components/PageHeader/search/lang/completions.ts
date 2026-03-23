import {
  type CompletionContext,
  type CompletionResult,
  type Completion,
} from '@codemirror/autocomplete';
import { syntaxTree } from '@codemirror/language';
import type { FieldDefinition } from '@/constants/fields';
import { OPERATORS } from '@/constants/operators';

const KNOWN_VALUES: Record<string, string[]> = {
  kind: ['Unspecified', 'Internal', 'Server', 'Client', 'Producer', 'Consumer'],
  statusCode: ['Unset', 'Ok', 'Error'],
  severityText: ['TRACE', 'DEBUG', 'INFO', 'WARN', 'ERROR', 'FATAL'],
};

export function createQueryCompletionSource(
  getFields: () => FieldDefinition[]
) {
  return function queryCompletionSource(
    context: CompletionContext
  ): CompletionResult | null {
    const tree = syntaxTree(context.state);
    const node = tree.resolveInner(context.pos, -1);
    const parentNode = node.parent;

    // Determine what to complete based on syntax tree position
    // After a logical operator or at the start of the query: suggest fields
    if (
      node.name === 'LogicalOp' ||
      node.name === 'Query' ||
      (parentNode?.name === 'Query' && context.pos === 0)
    ) {
      return fieldCompletions(context, getFields());
    }

    // Inside or after a group opening paren
    if (node.name === 'Group') {
      return fieldCompletions(context, getFields());
    }

    // After a field (inside a Comparison, before operator): suggest operators
    if (node.name === 'Field' && parentNode?.name === 'Comparison') {
      const fieldText = context.state.sliceDoc(node.from, node.to);
      return operatorCompletions(context, fieldText, getFields());
    }

    // After an operator: suggest values (or known enum values)
    if (
      (node.name === 'Operator' || node.name === 'KeywordOperator') &&
      parentNode?.name === 'Comparison'
    ) {
      const fieldNode = parentNode.getChild('Field');
      if (fieldNode) {
        const fieldText = context.state.sliceDoc(fieldNode.from, fieldNode.to);
        return valueCompletions(context, fieldText);
      }
    }

    // User is typing something that could be a field name
    const word = context.matchBefore(/[\w.]+/);
    if (word) {
      // Check if we're in a position where a field name is expected
      const beforeWord = context.state.sliceDoc(
        Math.max(0, word.from - 20),
        word.from
      );
      const trimmed = beforeWord.trimEnd();

      // After a logical op, after opening paren, or at start
      if (
        trimmed === '' ||
        /\b(AND|OR)\s*$/i.test(trimmed) ||
        trimmed.endsWith('(')
      ) {
        return fieldCompletions(context, getFields(), word.from);
      }

      // After an operator: suggest values
      if (/[=!><~^$]\s*$/.test(trimmed) || /\b(CONTAINS|IN|REGEXP|NOT IN|NOT CONTAINS)\s*$/i.test(trimmed)) {
        const line = context.state.sliceDoc(
          context.state.doc.lineAt(word.from).from,
          word.from
        );
        const fieldMatch = line.match(/([\w.]+)\s*(?:=|!=|>|<|>=|<=|=~|!~|\^|\$|CONTAINS|REGEXP|NOT CONTAINS|NOT IN|IN)\s*$/i);
        if (fieldMatch) {
          return valueCompletions(context, fieldMatch[1], word.from);
        }
      }
    }

    // Explicit activation (Ctrl+Space) with no word: suggest fields
    if (context.explicit) {
      return fieldCompletions(context, getFields());
    }

    return null;
  };
}

function fieldCompletions(
  context: CompletionContext,
  fields: FieldDefinition[],
  from?: number
): CompletionResult | null {
  const options: Completion[] = fields
    .filter((f): f is Exclude<FieldDefinition, { searchScope: 'global' }> => f.searchScope !== 'global')
    .map(f => ({
      label: f.name,
      type: 'property',
      detail: f.type,
      info: 'description' in f ? f.description : undefined,
      boost: f.searchScope === 'field' ? 1 : 0,
    }));

  if (options.length === 0) return null;

  return {
    from: from ?? context.pos,
    options,
    validFor: /^[\w.]*$/,
  };
}

function operatorCompletions(
  context: CompletionContext,
  fieldName: string,
  fields: FieldDefinition[]
): CompletionResult | null {
  const field = fields.find(
    f => f.searchScope !== 'global' && f.name.toLowerCase() === fieldName.toLowerCase()
  );

  const ops = field && field.searchScope !== 'global'
    ? field.operators
    : Object.values(OPERATORS);

  const options: Completion[] = ops.map(op => ({
    label: op.symbol,
    type: 'operator',
    detail: op.label,
  }));

  return {
    from: context.pos,
    options,
  };
}

function valueCompletions(
  context: CompletionContext,
  fieldName: string,
  from?: number
): CompletionResult | null {
  const knownValues = KNOWN_VALUES[fieldName];
  if (!knownValues) return null;

  const options: Completion[] = knownValues.map(v => ({
    label: v,
    type: 'enum',
  }));

  return {
    from: from ?? context.pos,
    options,
    validFor: /^[\w]*$/,
  };
}
