import type { SpanNode } from '@/types/api-types'
import {
  computeTreeMeta,
  isErrorSpan,
} from '@/components/traces/Waterfall/WaterfallView.svelte'
import { completionState, type groupResults } from './group-results'
import {
  estimatedCost,
  type EvalResult,
  type EvalExperiment,
  type TelemetryReference,
} from './model'
import {
  evalExecutionRows,
  referenceKey,
  resultTraceNodes,
  spanAttribute,
  resolveEvalReference,
  type indexEvalTraces,
} from './eval-traces'
import type { TreeConnectorMeta } from '@/components/shared/TreeGutter.svelte'

type Estimate = { total: number | null; incomplete: boolean }
export type TimeWindow = { startTime: bigint; endTime: bigint }
export type EvalOutlineRow = {
  id: string
  parentID: string | null
  kind:
    | 'experiment'
    | 'case'
    | 'result'
    | 'execution'
    | 'grading'
    | 'tool'
    | 'grader'
    | 'span'
  caseID: string
  result?: EvalResult
  name: string
  depth: number
  state: EvalResult['state'] | 'Unknown'
  internalFault: boolean
  spanError: boolean
  reference?: TelemetryReference
  window?: TimeWindow
  seconds?: number
  assessmentID?: string
  toolCounts?: { total: number; failed: number }
  estimate: Estimate
  tree: TreeConnectorMeta
}

export function combineEstimates(estimates: Estimate[]): Estimate {
  const known = estimates.filter(item => item.total !== null)
  return {
    total: known.length
      ? known.reduce((sum, item) => sum + item.total!, 0)
      : null,
    incomplete:
      known.length < estimates.length ||
      estimates.some(item => item.incomplete),
  }
}

const estimate = (value?: number): Estimate => ({
  total: value ?? null,
  incomplete: value === undefined,
})

export function spanWindow(
  nodes: (SpanNode | undefined)[]
): TimeWindow | undefined {
  return mergeWindows(nodes.map(node => node?.spanData))
}

function mergeWindows(
  windows: (TimeWindow | undefined)[]
): TimeWindow | undefined {
  let window: TimeWindow | undefined
  for (const span of windows) {
    if (!span || span.endTime < span.startTime) continue
    if (!window) window = { startTime: span.startTime, endTime: span.endTime }
    else {
      if (span.startTime < window.startTime) window.startTime = span.startTime
      if (span.endTime > window.endTime) window.endTime = span.endTime
    }
  }
  return window
}

/** Logical eval ownership supplies grouping; stored ancestry supplies tool connections.
 * Grouping IDs are display-only and never replace received trace/span identities. */
export function buildEvalOutline(
  groups: ReturnType<typeof groupResults>,
  experiment: EvalExperiment,
  index: ReturnType<typeof indexEvalTraces>
): EvalOutlineRow[] {
  const source = new Map(experiment.results.map(result => [result.id, result]))
  const nodeFor = (reference?: TelemetryReference) =>
    resolveEvalReference(reference, index)
  const rows: EvalOutlineRow[] = []
  const emptyTree = {
    childrenCount: 0,
    ancestorHasNextSibling: [],
    isLastChild: true,
  }
  // The experiment's totals, timing and execution state include every result, even
  // when search hides a case or repetition from the visible outline.
  const resultStatuses = new Map(
    experiment.results.map(result => {
      const execution = evalExecutionRows(result, index)
      const resultNode = nodeFor(result.resultTelemetry)
      const spanError =
        execution.spanError ||
        !!(resultNode && isErrorSpan(resultNode.spanData))
      return [
        result.id,
        {
          state: spanError ? ('Error' as const) : execution.state,
          spanError,
          internalFault: execution.internalFault,
          window:
            spanWindow([resultNode]) ??
            spanWindow([
              nodeFor(result.telemetry),
              ...execution.rows.map(row => row.node),
              ...resultTraceNodes(result, index),
            ]),
        },
      ]
    })
  )
  const allStatuses = [...resultStatuses.values()]
  const experimentNode = nodeFor(experiment.telemetry)
  const experimentSpanError =
    !!(experimentNode && isErrorSpan(experimentNode.spanData)) ||
    allStatuses.some(item => item.spanError)
  const experimentInternalFault =
    experiment.pauseReason === 'internal-error' ||
    allStatuses.some(item => item.internalFault)
  const experimentRow: EvalOutlineRow = {
    id: `experiment/${experiment.id}`,
    parentID: null,
    kind: 'experiment',
    caseID: '',
    name: experiment.name,
    depth: 0,
    state:
      experimentSpanError || experimentInternalFault
        ? 'Error'
        : experiment.state === 'Paused'
          ? 'Running'
          : experiment.state,
    internalFault: experimentInternalFault,
    spanError: experimentSpanError,
    reference: experiment.telemetry,
    window:
      spanWindow([experimentNode]) ??
      mergeWindows(allStatuses.map(item => item.window)),
    seconds: experiment.seconds,
    estimate: combineEstimates([
      estimatedCost(experiment.results, 'investigatorCost'),
      estimatedCost(experiment.results, 'graderCost'),
    ]),
    tree: emptyTree,
  }
  rows.push(experimentRow)
  for (const group of groups) {
    const caseRow: EvalOutlineRow = {
      id: `case/${group.id}`,
      parentID: experimentRow.id,
      kind: 'case',
      caseID: group.id,
      name: group.name,
      depth: 1,
      state: group.state,
      internalFault: false,
      spanError: false,
      estimate: combineEstimates([
        estimatedCost(group.allResults, 'investigatorCost'),
        estimatedCost(group.allResults, 'graderCost'),
      ]),
      tree: emptyTree,
    }
    rows.push(caseRow)
    // Hidden repetitions still contribute to the case's status and cost.
    const statuses = group.allResults.map(item => resultStatuses.get(item.id)!)
    caseRow.state = completionState(statuses)
    caseRow.spanError = statuses.some(item => item.spanError)
    caseRow.internalFault = statuses.some(item => item.internalFault)
    caseRow.window = mergeWindows(statuses.map(item => item.window))
    for (const item of group.results) {
      const result = source.get(item.id)!
      const execution = evalExecutionRows({ ...result, assessments: [] }, index)
      const grading = evalExecutionRows(
        { ...result, telemetry: undefined, tools: [], state: 'Complete' },
        index
      )
      const resultNode = nodeFor(result.resultTelemetry)
      const spanError =
        execution.spanError ||
        grading.spanError ||
        !!(resultNode && isErrorSpan(resultNode.spanData))
      const resultRow: EvalOutlineRow = {
        id: `result/${result.id}`,
        parentID: caseRow.id,
        kind: 'result',
        caseID: group.id,
        result,
        name: 'Result',
        depth: 2,
        state: spanError ? 'Error' : result.state,
        internalFault: result.state === 'Error',
        spanError,
        reference: result.resultTelemetry,
        window:
          spanWindow([resultNode]) ??
          spanWindow([
            nodeFor(result.telemetry),
            ...execution.rows.map(row => row.node),
            ...grading.rows.map(row => row.node),
            ...resultTraceNodes(result, index),
          ]),
        seconds: result.seconds,
        estimate: combineEstimates([
          estimate(result.investigatorCost),
          estimate(result.graderCost),
        ]),
        tree: emptyTree,
      }
      // A result without recorded repetition identity belongs directly on its
      // test case. This is a display projection, not an invented Result 1.
      const owner =
        result.repetition === undefined && group.allResults.length === 1
          ? caseRow
          : resultRow
      if (owner === caseRow) {
        Object.assign(caseRow, {
          result,
          reference: result.resultTelemetry ?? result.telemetry,
          window: resultRow.window,
          seconds: result.seconds,
        })
      } else {
        resultRow.reference ??= result.telemetry
        rows.push(resultRow)
      }
      const nativeNodes = resultTraceNodes(result, index)
      if (nativeNodes.length) {
        owner.toolCounts = { total: 0, failed: 0 }
        const root = resolveEvalReference(result.telemetry, index)?.spanData
        if (root)
          owner.reference = { traceID: root.traceID, spanID: root.spanID }
        const parents = new Map<string, EvalOutlineRow>(
          root ? [[referenceKey(root), owner]] : []
        )
        for (const node of nativeNodes) {
          const span = node.spanData
          if (root && referenceKey(span) === referenceKey(root)) continue
          const recordedParent = span.parentSpanID
            ? parents.get(
                referenceKey({
                  traceID: span.traceID,
                  spanID: span.parentSpanID,
                })
              )
            : undefined
          const parent = recordedParent ?? owner
          const role = spanAttribute(node, 'promptfoo.span.role')
          const kind =
            spanAttribute(node, 'gen_ai.operation.name') === 'execute_tool'
              ? 'tool'
              : role === 'grader'
                ? 'grader'
                : role === 'target'
                  ? 'execution'
                  : 'span'
          const child: EvalOutlineRow = {
            id: `${result.id}/span/${referenceKey(span)}`,
            parentID: parent.id,
            kind,
            caseID: group.id,
            result,
            name: span.name,
            assessmentID:
              result.assessments.find(
                assessment =>
                  assessment.telemetry &&
                  referenceKey(assessment.telemetry) === referenceKey(span)
              )?.id ?? (kind === 'grader' ? parent.assessmentID : undefined),
            depth: parent.depth + 1,
            state: isErrorSpan(span) ? 'Error' : 'Complete',
            internalFault: false,
            toolCounts: {
              total: kind === 'tool' ? 1 : 0,
              failed: kind === 'tool' && isErrorSpan(span) ? 1 : 0,
            },
            spanError: isErrorSpan(span),
            reference: { traceID: span.traceID, spanID: span.spanID },
            window: spanWindow([node]),
            estimate: estimate(),
            tree: {
              ...emptyTree,
              brokenParent: !!span.parentSpanID && !recordedParent,
            },
          }
          parents.set(referenceKey(span), child)
          rows.push(child)
        }
        continue
      }
      // Roles are attributes. A separate execution row requires a distinct,
      // explicitly referenced span; scores alone do not create grading spans.
      const phases = [
        { kind: 'execution' as const, result: execution },
        { kind: 'grading' as const, result: grading },
      ]
      for (const phase of phases) {
        let parent = owner
        if (
          phase.kind === 'execution' &&
          result.resultTelemetry &&
          result.telemetry?.spanID &&
          referenceKey(result.resultTelemetry) !==
            referenceKey(result.telemetry)
        ) {
          parent = {
            id: `execution/${result.id}`,
            parentID: owner.id,
            kind: 'execution',
            caseID: group.id,
            result,
            name: nodeFor(result.telemetry)?.spanData.name ?? 'Execution',
            depth: owner.depth + 1,
            reference: result.telemetry,
            window: spanWindow([nodeFor(result.telemetry)]),
            seconds: result.seconds,
            state: execution.state,
            internalFault: execution.internalFault,
            spanError: execution.spanError,
            estimate: estimate(result.investigatorCost),
            tree: emptyTree,
          }
          rows.push(parent)
        }
        const parents = [parent.id]
        for (const child of phase.result.rows) {
          const parentID = parents[child.depth - 1]!
          parents[child.depth] = child.id
          parents.length = child.depth + 1
          rows.push({
            id: child.id,
            parentID,
            kind: phase.kind === 'execution' ? 'tool' : 'grader',
            caseID: group.id,
            result,
            name: child.name,
            depth: child.depth + parent.depth,
            state: child.state,
            internalFault: false,
            spanError: child.state === 'Error',
            reference: child.reference,
            window: spanWindow([child.node]),
            seconds: child.seconds,
            assessmentID: child.assessmentID,
            estimate: estimate(child.cost),
            // Grading is a logical group, not a claim that the investigator is its parent.
            tree: {
              ...emptyTree,
              brokenParent:
                phase.kind === 'execution' || child.depth > 1
                  ? child.tree.brokenParent
                  : false,
            },
          })
        }
      }
      if (group.allResults.length === 1) {
        caseRow.window = resultRow.window
        caseRow.seconds = resultRow.seconds
      }
    }
  }
  // Resolve missing containers bottom-up. A valid recorded span wins even when
  // a child's clock extends beyond it. Durations without starts cannot locate
  // a child on the timeline and must not be summed into an elapsed window.
  const childWindows = new Map<string, TimeWindow[]>()
  for (const row of [...rows].reverse()) {
    row.window ??= mergeWindows(childWindows.get(row.id) ?? [])
    if (row.parentID && row.window) {
      const siblings = childWindows.get(row.parentID) ?? []
      siblings.push(row.window)
      childWindows.set(row.parentID, siblings)
    }
  }
  const byID = new Map(rows.map(row => [row.id, row]))
  for (const row of [...rows].reverse()) {
    const parent = row.parentID ? byID.get(row.parentID) : undefined
    if (parent?.toolCounts && row.toolCounts) {
      parent.toolCounts.total += row.toolCounts.total
      parent.toolCounts.failed += row.toolCounts.failed
    }
    if (parent && row.spanError) {
      parent.spanError = true
      parent.state = 'Error'
    }
  }
  const metadata = computeTreeMeta(
    rows.map(row => ({
      depth: row.depth,
      spanData: { spanID: row.id, parentSpanID: row.parentID },
    }))
  )
  return rows.map((row, index) => ({
    ...row,
    tree: { ...metadata[index]!, brokenParent: row.tree.brokenParent ?? false },
  }))
}

export function outlineOpen(
  row: EvalOutlineRow,
  views: readonly string[],
  overrides: Record<string, boolean>
) {
  return (
    overrides[row.id] ??
    (row.kind === 'experiment'
      ? true
      : row.kind === 'case'
        ? views.includes('results') || views.includes('time')
        : row.kind === 'result' ||
            row.kind === 'execution' ||
            row.kind === 'span'
          ? views.includes('time')
          : false)
  )
}

export function visibleOutlineRows(
  rows: EvalOutlineRow[],
  views: readonly string[],
  overrides: Record<string, boolean>
) {
  let hiddenBelow = Infinity
  return rows.filter(row => {
    if (row.depth > hiddenBelow) return false
    hiddenBelow =
      row.tree.childrenCount && !outlineOpen(row, views, overrides)
        ? row.depth
        : Infinity
    return true
  })
}
