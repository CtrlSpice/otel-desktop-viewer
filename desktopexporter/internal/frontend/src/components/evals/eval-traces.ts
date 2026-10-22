import type { SpanNode, TraceData } from '@/types/api-types'
import {
  computeTreeMeta,
  isErrorSpan,
} from '@/components/traces/Waterfall/WaterfallView.svelte'
import type { TreeConnectorMeta } from '@/components/shared/TreeGutter.svelte'
import type { EvalResult, EvalExperiment, TelemetryReference } from './model'

export const referenceKey = (reference: TelemetryReference) =>
  `${reference.traceID.toLowerCase()}/${reference.spanID?.toLowerCase() ?? ''}`

export function evalTraceReferences(experiment: EvalExperiment) {
  return [
    experiment.telemetry,
    ...experiment.results.flatMap(result => [
      result.telemetry,
      result.resultTelemetry,
      ...result.tools.map(tool => tool.telemetry),
      ...result.assessments.map(assessment => assessment.telemetry),
    ]),
  ].flatMap(reference => {
    if (!reference) return []
    const identity: TelemetryReference = { traceID: reference.traceID }
    if (reference.spanID) identity.spanID = reference.spanID
    return [identity]
  })
}

export function indexEvalTraces(traces: TraceData[]) {
  const nodes = new Map<string, SpanNode>()
  const roots = new Map<string, SpanNode[]>()
  const parents = new Map<string, string>()
  const order = new Map<string, number>()
  for (const trace of traces) {
    const stack: string[] = []
    for (const node of trace.spans) {
      const key = referenceKey(node.spanData)
      nodes.set(key, node)
      if (!node.spanData.parentSpanID) {
        const traceID = node.spanData.traceID.toLowerCase()
        roots.set(traceID, [...(roots.get(traceID) ?? []), node])
      }
      order.set(key, order.size)
      const parent = node.depth > 0 ? stack[node.depth - 1] : undefined
      if (parent) parents.set(key, parent)
      stack[node.depth] = key
      stack.length = node.depth + 1
    }
  }
  return { nodes, parents, order, roots }
}

/** A trace-only reference resolves only to one recorded root, never the first row. */
export function resolveEvalReference(
  reference: TelemetryReference | undefined,
  index: ReturnType<typeof indexEvalTraces>
) {
  if (!reference) return undefined
  if (reference.spanID) return index.nodes.get(referenceKey(reference))
  const roots = index.roots.get(reference.traceID.toLowerCase()) ?? []
  return roots.length === 1 ? roots[0] : undefined
}

export function spanAttribute(node: SpanNode, key: string): string | undefined {
  const attributes = node.spanData.attributes.filter(item => item.key === key)
  const attribute = attributes.length === 1 ? attributes[0] : undefined
  return attribute?.value.kind === 'string' && !attribute.hasConflict
    ? attribute.value.value
    : undefined
}

/** Available trace branches, preserving missing parent IDs and start-ordered siblings. */
export function resultTraceNodes(
  result: EvalResult,
  index: ReturnType<typeof indexEvalTraces>
): SpanNode[] {
  if (!result.telemetry || result.telemetry.spanID) return []
  const nodes = [...index.nodes.values()].filter(
    node =>
      node.spanData.traceID.toLowerCase() ===
      result.telemetry!.traceID.toLowerCase()
  )
  const ids = new Set(nodes.map(node => node.spanData.spanID))
  const children = new Map<string | null, SpanNode[]>()
  for (const node of nodes) {
    const parent = node.spanData.parentSpanID
    const key = parent && ids.has(parent) ? parent : null
    children.set(key, [...(children.get(key) ?? []), node])
  }
  const ordered: SpanNode[] = []
  const visited = new Set<string>()
  const visit = (node: SpanNode) => {
    if (visited.has(node.spanData.spanID)) return
    visited.add(node.spanData.spanID)
    ordered.push(node)
    for (const child of siblings(node.spanData.spanID)) visit(child)
  }
  const siblings = (id: string | null) =>
    (children.get(id) ?? []).sort((a, b) =>
      a.spanData.startTime < b.spanData.startTime
        ? -1
        : a.spanData.startTime > b.spanData.startTime
          ? 1
          : 0
    )
  for (const node of siblings(null)) visit(node)
  for (const node of nodes) visit(node)
  return ordered
}

/** Enrich display data from received spans without altering the native export. */
export function projectEvalTraceEvidence(
  experiment: EvalExperiment,
  index: ReturnType<typeof indexEvalTraces>
): EvalExperiment {
  return {
    ...experiment,
    results: experiment.results.map(result => {
      const nodes = resultTraceNodes(result, index)
      if (!nodes.length) return result
      const tools = [...result.tools]
      const known = new Set(
        tools.flatMap(tool =>
          tool.telemetry ? [referenceKey(tool.telemetry)] : []
        )
      )
      for (const node of nodes) {
        if (
          spanAttribute(node, 'gen_ai.operation.name') !== 'execute_tool' ||
          known.has(referenceKey(node.spanData))
        )
          continue
        tools.push({
          name: spanAttribute(node, 'gen_ai.tool.name') ?? node.spanData.name,
          state: isErrorSpan(node.spanData) ? 'Error' : 'OK',
          message: node.spanData.statusMessage || undefined,
          telemetry: {
            traceID: node.spanData.traceID,
            spanID: node.spanData.spanID,
          },
        })
        known.add(referenceKey(node.spanData))
      }
      const root = resolveEvalReference(result.telemetry, index)?.spanData
      const assessments = result.assessments.map(assessment => {
        if (
          assessment.telemetry ||
          !assessment.sourceName ||
          result.assessments.filter(
            item => item.sourceName === assessment.sourceName
          ).length !== 1
        )
          return assessment
        const graders = nodes.filter(
          node =>
            spanAttribute(node, 'promptfoo.span.role') === 'grader' &&
            spanAttribute(node, 'promptfoo.grader.id') ===
              assessment.sourceName &&
            spanAttribute(node, 'gen_ai.evaluation.name') ===
              assessment.sourceName
        )
        if (graders.length !== 1) return assessment
        const span = graders[0]!.spanData
        return {
          ...assessment,
          telemetry: { traceID: span.traceID, spanID: span.spanID },
        }
      })
      return {
        ...result,
        assessments,
        resultTelemetry:
          result.resultTelemetry ??
          (root ? { traceID: root.traceID, spanID: root.spanID } : undefined),
        tools,
        toolsKnown: result.toolsKnown || tools.length > 0,
      }
    }),
  }
}

export type EvalExecutionRow = {
  id: string
  name: string
  reference?: TelemetryReference
  seconds?: number
  cost?: number
  assessmentID?: string
  state: EvalResult['state']
  node?: SpanNode
  depth: number
  tree: TreeConnectorMeta
}

/** Retain eval evidence rows, but connect stored references by actual ancestry. */
export function evalExecutionRows(
  result: EvalResult,
  index: ReturnType<typeof indexEvalTraces>
) {
  const items = [
    ...result.tools.map((tool, position) => ({
      id: `${result.id}/tool/${position}`,
      name: tool.name,
      reference: tool.telemetry,
      seconds: tool.seconds,
      cost: tool.cost,
      state:
        tool.state === 'Error' ? ('Error' as const) : ('Complete' as const),
    })),
    ...result.assessments
      .filter(item => item.telemetry)
      .map(item => ({
        id: `${result.id}/assessment/${item.id}`,
        name: `${item.evaluator || 'Grader'} · ${item.id}`,
        reference: item.telemetry,
        seconds: undefined,
        cost: item.cost,
        assessmentID: item.id,
        state: 'Complete' as const,
      })),
  ]
  const resultKey = result.telemetry
    ? referenceKey(result.telemetry)
    : undefined
  const identities = new Map<string, string>()
  for (const item of items) {
    if (item.reference?.spanID) {
      const key = referenceKey(item.reference)
      if (key !== resultKey && !identities.has(key))
        identities.set(key, item.id)
    }
  }
  const children = new Map<string, typeof items>()
  for (const item of items) {
    let parentID = result.id
    let parent = item.reference
      ? index.parents.get(referenceKey(item.reference))
      : undefined
    while (parent && parent !== resultKey) {
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
  visit(result.id, 1)
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
        spanID: resultKey ?? result.id,
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
  const resultNode = resultKey ? index.nodes.get(resultKey) : undefined
  const spanError =
    rows.some(row => row.state === 'Error') ||
    !!(resultNode && isErrorSpan(resultNode.spanData))
  return {
    internalFault: result.state === 'Error',
    spanError,
    state:
      result.state === 'Error' || spanError ? ('Error' as const) : result.state,
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
