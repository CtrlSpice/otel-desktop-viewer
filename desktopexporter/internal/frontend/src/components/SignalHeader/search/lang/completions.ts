import {
  type CompletionContext,
  type CompletionResult,
  type Completion,
} from '@codemirror/autocomplete';
import { syntaxTree } from '@codemirror/language';
import type { SyntaxNode } from '@lezer/common';
import type { FieldDefinition } from '@/constants/fields';
import { OPERATORS } from '@/constants/operators';
import {
  Array as ArrayTerm,
  Field as FieldTerm,
  KeywordOperator,
  Null,
  Operator as OperatorTerm,
  QuotedString,
  Value as ValueTerm,
} from './query.parser.terms';

const LOGICAL_COMPLETIONS: Completion[] = [
  {
    label: 'AND',
    type: 'keyword',
  },
  {
    label: 'OR',
    type: 'keyword',
  },
];

function logicalCompletionsFrom(from: number): CompletionResult {
  return {
    from,
    options: LOGICAL_COMPLETIONS,
    validFor: /^\s*(AND|OR)?$/i,
  };
}

function findAncestor(node: SyntaxNode, name: string): SyntaxNode | null {
  let n: SyntaxNode | null = node;
  while (n) {
    if (n.name === name) return n;
    n = n.parent;
  }
  return null;
}

function logCompletionDebug(
  context: CompletionContext,
  node: SyntaxNode
): void {
  if (!import.meta.env.DEV) return;
  const parents: string[] = [];
  let p: SyntaxNode | null = node;
  for (let i = 0; i < 10 && p; i++) {
    parents.push(p.name);
    p = p.parent;
  }
  const lo = Math.max(0, context.pos - 48);
  const hi = Math.min(context.state.doc.length, context.pos + 12);
  console.debug('[search completion]', {
    pos: context.pos,
    node: node.name,
    parents,
    snippet: context.state.sliceDoc(lo, hi),
  });
}

function getValueNode(comparison: SyntaxNode): SyntaxNode | null {
  return (
    comparison.getChild(ValueTerm) ??
    comparison.getChild(QuotedString) ??
    comparison.getChild(ArrayTerm) ??
    comparison.getChild(Null)
  );
}

function getOperatorNode(comparison: SyntaxNode): SyntaxNode | null {
  return (
    comparison.getChild(OperatorTerm) ??
    comparison.getChild(KeywordOperator)
  );
}

/** Heuristic: raw hex pasted / typed that looks like a trace or span id → suggest a full condition. */
function idPatternCompletions(context: CompletionContext): CompletionResult | null {
  const word = context.matchBefore(/[a-fA-F0-9]+/);
  if (!word || word.from === word.to) return null;
  const raw = word.text;
  if (!/^[a-fA-F0-9]+$/.test(raw)) return null;

  const options: Completion[] = [];
  if (raw.length === 32) {
    options.push({
      label: `traceID = ${raw}`,
      type: 'text',
      apply: `traceID = ${raw}`,
    });
  }
  if (raw.length === 16) {
    options.push({
      label: `spanID = ${raw}`,
      type: 'text',
      apply: `spanID = ${raw}`,
    });
  }

  if (options.length === 0) return null;

  return {
    from: word.from,
    to: word.to,
    options,
  };
}

export function createQueryCompletionSource(
  getFields: () => FieldDefinition[]
) {
  return function queryCompletionSource(
    context: CompletionContext
  ): CompletionResult | null {
    const tree = syntaxTree(context.state);
    const node = tree.resolveInner(context.pos, -1);
    logCompletionDebug(context, node);

    // Prefer id-shape completions when the token is pure hex (paste-friendly).
    const idHit = idPatternCompletions(context);
    if (idHit) return idHit;

    const comparison = findAncestor(node, 'Comparison');

    if (comparison) {
      const field = comparison.getChild(FieldTerm);
      const opNode = getOperatorNode(comparison);
      const valueNode = getValueNode(comparison);
      const pos = context.pos;

      // Still in or at end of field name → complete field names, not operators.
      if (field && pos <= field.to) {
        return fieldCompletions(context, getFields(), field.from);
      }

      // After field: whitespace before operator → operators (not another field).
      if (field && pos > field.to) {
        const between = context.state.sliceDoc(field.to, pos);
        if (/^\s*$/.test(between)) {
          if (!opNode || pos < opNode.from) {
            return operatorCompletions(
              context,
              context.state.sliceDoc(field.from, field.to),
              getFields()
            );
          }
        }
      }

      if (
        (node.name === 'Operator' || node.name === 'KeywordOperator') &&
        findAncestor(node, 'Comparison') === comparison
      ) {
        const fieldNode = comparison.getChild(FieldTerm);
        if (fieldNode) {
          const fieldText = context.state.sliceDoc(fieldNode.from, fieldNode.to);
          return valueCompletions(context, fieldText, getFields());
        }
      }

      // Cursor in value position (inside or at end of value token)
      if (valueNode && pos >= valueNode.from && pos <= valueNode.to) {
        const fieldNode = comparison.getChild(FieldTerm);
        if (fieldNode) {
          const fieldText = context.state.sliceDoc(fieldNode.from, fieldNode.to);
          return valueCompletions(
            context,
            fieldText,
            getFields(),
            valueNode.from
          );
        }
      }

      // After a complete value: AND / OR (space after value, or end of input).
      if (field && opNode && valueNode && pos >= valueNode.to) {
        const gap = context.state.sliceDoc(valueNode.to, pos);
        const atDocEnd = pos === context.state.doc.length;
        if (/^\s*$/.test(gap) && (gap.length >= 1 || atDocEnd)) {
          return logicalCompletionsFrom(pos);
        }
      }
    }

    const groupNode = findAncestor(node, 'Group');
    if (groupNode && context.pos >= groupNode.to) {
      const afterGroup = context.state.sliceDoc(groupNode.to, context.pos);
      if (/^\s*$/.test(afterGroup)) {
        return logicalCompletionsFrom(context.pos);
      }
    }

    // After logical op: fields.
    if (node.name === 'LogicalOp') {
      return fieldCompletions(context, getFields());
    }

    if (node.name === 'Query') {
      return fieldCompletions(context, getFields());
    }

    if (node.name === 'Group' && context.pos < node.to) {
      return fieldCompletions(context, getFields());
    }

    const parentNode = node.parent;

    if (
      node.name === 'Field' &&
      parentNode?.name === 'Comparison' &&
      context.pos > node.to
    ) {
      const fieldText = context.state.sliceDoc(node.from, node.to);
      return operatorCompletions(context, fieldText, getFields());
    }

    const word = context.matchBefore(/[\w.]+/);
    if (word) {
      const beforeWord = context.state.sliceDoc(
        Math.max(0, word.from - 20),
        word.from
      );
      const trimmed = beforeWord.trimEnd();

      if (
        trimmed === '' ||
        /\b(AND|OR)\s*$/i.test(trimmed) ||
        trimmed.endsWith('(')
      ) {
        return fieldCompletions(context, getFields(), word.from);
      }

      if (
        /=|!|>|<|~|\^|\$/.test(trimmed.slice(-1)) ||
        /\b(CONTAINS|IN|REGEXP|NOT IN|NOT CONTAINS)\s*$/i.test(trimmed)
      ) {
        const line = context.state.sliceDoc(
          context.state.doc.lineAt(word.from).from,
          word.from
        );
        const fieldMatch = line.match(
          /([\w.]+)\s*(?:=|!=|>|<|>=|<=|=~|!~|\^|\$|CONTAINS|REGEXP|NOT CONTAINS|NOT IN|IN)\s*$/i
        );
        if (fieldMatch) {
          return valueCompletions(
            context,
            fieldMatch[1],
            getFields(),
            word.from
          );
        }
      }
    }

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
    .filter(
      (f): f is Exclude<FieldDefinition, { searchScope: 'global' }> =>
        f.searchScope !== 'global'
    )
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
    f =>
      f.searchScope !== 'global' &&
      f.name.toLowerCase() === fieldName.toLowerCase()
  );

  const ops =
    field && field.searchScope !== 'global'
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
  fields: FieldDefinition[],
  from?: number
): CompletionResult | null {
  const field = fields.find(
    f =>
      f.searchScope !== 'global' &&
      f.name.toLowerCase() === fieldName.toLowerCase()
  );
  if (!field || field.searchScope === 'global') return null;

  const knownValues =
    'enumValues' in field && field.enumValues && field.enumValues.length > 0
      ? field.enumValues
      : null;
  if (!knownValues) return null;

  const options: Completion[] = [...knownValues].map(v => ({
    label: v,
    type: 'enum',
  }));

  return {
    from: from ?? context.pos,
    options,
    validFor: /^[\w]*$/,
  };
}
