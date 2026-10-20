import type { SpanNode, TraceData } from '@/types/api-types'
import {
  computeTreeMeta,
  isErrorSpan,
} from '@/components/traces/Waterfall/WaterfallView.svelte'
import type { TreeConnectorMeta } from '@/components/shared/TreeGutter.svelte'
import type { EvalAttempt, EvalRun, TelemetryReference } from './model'

export const referenceKey = (reference: TelemetryReference) =>
  `${reference.traceID.toLowerCase()}/${reference.spanID?.toLowerCase() ?? ''}`

export function evalTraceReferences(run: EvalRun) {
  return [
    run.telemetry,
    ...run.attempts.flatMap(attempt => [
      attempt.telemetry,
      attempt.attemptTelemetry,
      ...attempt.tools.map(tool => tool.telemetry),
      ...attempt.assessments.map(assessment => assessment.telemetry),
    ]),
  ].flatMap(reference =>
    reference?.spanID
      ? [{ traceID: reference.traceID, spanID: reference.spanID }]
      : []
  )
}

export function indexEvalTraces(traces: TraceData[]) {
  const nodes = new Map<string, SpanNode>()
  const parents = new Map<string, string>()
  const order = new Map<string, number>()
  for (const trace of traces) {
    const stack: string[] = []
    for (const node of trace.spans) {
      const key = referenceKey(node.spanData)
      nodes.set(key, node)
      order.set(key, order.size)
      const parent = node.depth > 0 ? stack[node.depth - 1] : undefined
      if (parent) parents.set(key, parent)
      stack[node.depth] = key
      stack.length = node.depth + 1
    }
  }
  return { nodes, parents, order }
}

export type EvalExecutionRow = {
  id: string
  name: string
  reference?: TelemetryReference
  seconds?: number
  cost?: number
  assessmentID?: string
  state: EvalAttempt['state']
  node?: SpanNode
  depth: number
  tree: TreeConnectorMeta
}

/** Retain eval evidence rows, but connect stored references by actual ancestry. */
export function evalExecutionRows(
  attempt: EvalAttempt,
  index: ReturnType<typeof indexEvalTraces>
) {
  const items = [
    ...attempt.tools.map((tool, position) => ({
      id: `${attempt.id}/tool/${position}`,
      name: tool.name,
      reference: tool.telemetry,
      seconds: tool.seconds,
      cost: tool.cost,
      state:
        tool.state === 'Error' ? ('Error' as const) : ('Complete' as const),
    })),
    ...attempt.assessments.map(item => ({
      id: `${attempt.id}/assessment/${item.id}`,
      name: `${item.evaluator || 'Grader'} · ${item.id}`,
      reference: item.telemetry,
      seconds: undefined,
      cost: item.cost,
      assessmentID: item.id,
      state: 'Complete' as const,
    })),
  ]
  const attemptKey = attempt.telemetry
    ? referenceKey(attempt.telemetry)
    : undefined
  const identities = new Map<string, string>()
  for (const item of items) {
    if (item.reference?.spanID) {
      const key = referenceKey(item.reference)
      if (key !== attemptKey && !identities.has(key))
        identities.set(key, item.id)
    }
  }
  const children = new Map<string, typeof items>()
  for (const item of items) {
    let parentID = attempt.id
    let parent = item.reference
      ? index.parents.get(referenceKey(item.reference))
      : undefined
    while (parent && parent !== attemptKey) {
      const ancestor = identities.get(parent)
      if (ancestor) {
        parentID = ancestor
        break
      }
      parent = index.parents.get(parent)
    }
    const siblings = children.get(parentID) ?? []
    siblings.push(item)
    children.set(parentID, siblings)
  }
  const rows: EvalExecutionRow[] = []
  function visit(parent: string, depth: number) {
    const siblings = children.get(parent) ?? []
    siblings.sort(
      (a, b) =>
        (a.reference
          ? (index.order.get(referenceKey(a.reference)) ?? Infinity)
          : Infinity) -
        (b.reference
          ? (index.order.get(referenceKey(b.reference)) ?? Infinity)
          : Infinity)
    )
    for (const item of siblings) {
      rows.push({
        ...item,
        node: item.reference
          ? index.nodes.get(referenceKey(item.reference))
          : undefined,
        depth,
        tree: {
          childrenCount: 0,
          ancestorHasNextSibling: [],
          isLastChild: true,
        },
      })
      visit(item.id, depth + 1)
    }
  }
  visit(attempt.id, 1)
  // Propagate execution errors through the displayed ancestry without changing
  // the recorded eval result or assessment score.
  const errorAtDepth: boolean[] = []
  for (let position = rows.length - 1; position >= 0; position--) {
    const row = rows[position]!
    const error =
      row.state === 'Error' ||
      (row.node && isErrorSpan(row.node.spanData)) ||
      errorAtDepth[row.depth + 1]
    if (error) row.state = 'Error'
    errorAtDepth.length = row.depth + 1
    errorAtDepth[row.depth] = !!errorAtDepth[row.depth] || !!error
  }
  const metadata = computeTreeMeta([
    {
      depth: 0,
      spanData: {
        spanID: attemptKey ?? attempt.id,
        parentSpanID: null,
      },
    },
    ...rows.map(row => ({
      depth: row.depth,
      spanData: {
        // Composite keys prevent borrowed ancestry when two traces reuse a span ID.
        spanID: row.reference ? referenceKey(row.reference) : row.id,
        parentSpanID: row.node?.spanData.parentSpanID
          ? referenceKey({
              traceID: row.node.spanData.traceID,
              spanID: row.node.spanData.parentSpanID,
            })
          : null,
      },
    })),
  ])
  const attemptNode = attemptKey ? index.nodes.get(attemptKey) : undefined
  const spanError =
    rows.some(row => row.state === 'Error') ||
    !!(attemptNode && isErrorSpan(attemptNode.spanData))
  return {
    internalFault: attempt.state === 'Error',
    spanError,
    state:
      attempt.state === 'Error' || spanError
        ? ('Error' as const)
        : attempt.state,
    childrenCount: metadata[0]!.childrenCount,
    rows: rows.map((row, position) => ({
      ...row,
      tree: metadata[position + 1]!,
    })),
  }
}

export function visibleExecutionRows(
  rows: EvalExecutionRow[],
  collapsed: readonly string[]
) {
  let hiddenBelow = Infinity
  return rows.filter(row => {
    if (row.depth > hiddenBelow) return false
    hiddenBelow = collapsed.includes(row.id) ? row.depth : Infinity
    return true
  })
}
