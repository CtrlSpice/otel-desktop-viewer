import type { SpanNode } from '@/types/api-types'
import {
  computeTreeMeta,
  isErrorSpan,
} from '@/components/traces/Waterfall/WaterfallView.svelte'
import { completionState, type groupReps } from './group-reps'
import {
  estimatedCost,
  type EvalRep,
  type EvalRun,
  type TelemetryReference,
} from './model'
import {
  evalExecutionRows,
  referenceKey,
  type indexEvalTraces,
} from './eval-traces'
import type { TreeConnectorMeta } from '@/components/shared/TreeGutter.svelte'

type Estimate = { total: number | null; incomplete: boolean }
export type TimeWindow = { startTime: bigint; endTime: bigint }
export type EvalOutlineRow = {
  id: string
  parentID: string | null
  kind: 'run' | 'case' | 'rep' | 'execution' | 'grading' | 'tool' | 'grader'
  caseID: string
  rep?: EvalRep
  name: string
  depth: number
  state: EvalRep['state'] | 'Unknown'
  internalFault: boolean
  spanError: boolean
  reference?: TelemetryReference
  window?: TimeWindow
  seconds?: number
  assessmentID?: string
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
  groups: ReturnType<typeof groupReps>,
  run: EvalRun,
  index: ReturnType<typeof indexEvalTraces>
): EvalOutlineRow[] {
  const source = new Map(run.reps.map(rep => [rep.id, rep]))
  const nodeFor = (reference?: TelemetryReference) =>
    reference ? index.nodes.get(referenceKey(reference)) : undefined
  const rows: EvalOutlineRow[] = []
  const emptyTree = {
    childrenCount: 0,
    ancestorHasNextSibling: [],
    isLastChild: true,
  }
  // The run's totals, timing and execution state include every rep, even
  // when search hides a case or repetition from the visible outline.
  const repStatuses = new Map(
    run.reps.map(rep => {
      const execution = evalExecutionRows(rep, index)
      const repNode = nodeFor(rep.repTelemetry)
      const spanError =
        execution.spanError || !!(repNode && isErrorSpan(repNode.spanData))
      return [
        rep.id,
        {
          state: spanError ? ('Error' as const) : execution.state,
          spanError,
          internalFault: execution.internalFault,
          window:
            spanWindow([repNode]) ??
            spanWindow([
              nodeFor(rep.telemetry),
              ...execution.rows.map(row => row.node),
            ]),
        },
      ]
    })
  )
  const allStatuses = [...repStatuses.values()]
  const runNode = nodeFor(run.telemetry)
  const runSpanError =
    !!(runNode && isErrorSpan(runNode.spanData)) ||
    allStatuses.some(item => item.spanError)
  const runInternalFault =
    run.pauseReason === 'internal-error' ||
    allStatuses.some(item => item.internalFault)
  const runRow: EvalOutlineRow = {
    id: `run/${run.id}`,
    parentID: null,
    kind: 'run',
    caseID: '',
    name: run.name,
    depth: 0,
    state:
      runSpanError || runInternalFault
        ? 'Error'
        : run.state === 'Paused'
          ? 'Running'
          : run.state,
    internalFault: runInternalFault,
    spanError: runSpanError,
    reference: run.telemetry,
    window:
      spanWindow([runNode]) ??
      mergeWindows(allStatuses.map(item => item.window)),
    seconds: run.seconds,
    estimate: combineEstimates([
      estimatedCost(run.reps, 'investigatorCost'),
      estimatedCost(run.reps, 'graderCost'),
    ]),
    tree: emptyTree,
  }
  rows.push(runRow)
  for (const group of groups) {
    const caseRow: EvalOutlineRow = {
      id: `case/${group.id}`,
      parentID: runRow.id,
      kind: 'case',
      caseID: group.id,
      name: group.name,
      depth: 1,
      state: group.state,
      internalFault: false,
      spanError: false,
      estimate: combineEstimates([
        estimatedCost(group.allReps, 'investigatorCost'),
        estimatedCost(group.allReps, 'graderCost'),
      ]),
      tree: emptyTree,
    }
    rows.push(caseRow)
    // Hidden repetitions still contribute to the case's status and cost.
    const statuses = group.allReps.map(item => repStatuses.get(item.id)!)
    caseRow.state = completionState(statuses)
    caseRow.spanError = statuses.some(item => item.spanError)
    caseRow.internalFault = statuses.some(item => item.internalFault)
    caseRow.window = mergeWindows(statuses.map(item => item.window))
    for (const item of group.reps) {
      const rep = source.get(item.id)!
      const execution = evalExecutionRows({ ...rep, assessments: [] }, index)
      const grading = evalExecutionRows(
        { ...rep, telemetry: undefined, tools: [], state: 'Complete' },
        index
      )
      const repNode = nodeFor(rep.repTelemetry)
      const spanError =
        execution.spanError ||
        grading.spanError ||
        !!(repNode && isErrorSpan(repNode.spanData))
      const repRow: EvalOutlineRow = {
        id: `rep/${rep.id}`,
        parentID: caseRow.id,
        kind: 'rep',
        caseID: group.id,
        rep,
        name: 'Result',
        depth: 2,
        state: spanError ? 'Error' : rep.state,
        internalFault: rep.state === 'Error',
        spanError,
        reference: rep.repTelemetry,
        window:
          spanWindow([repNode]) ??
          spanWindow([
            nodeFor(rep.telemetry),
            ...execution.rows.map(row => row.node),
            ...grading.rows.map(row => row.node),
          ]),
        seconds: rep.seconds,
        estimate: combineEstimates([
          estimate(rep.investigatorCost),
          estimate(rep.graderCost),
        ]),
        tree: emptyTree,
      }
      // A result without recorded repetition identity belongs directly on its
      // test case. This is a display projection, not an invented Rep 1.
      const owner =
        rep.repetition === undefined && group.allReps.length === 1
          ? caseRow
          : repRow
      if (owner === caseRow) {
        Object.assign(caseRow, {
          rep,
          reference: rep.repTelemetry ?? rep.telemetry,
          window: repRow.window,
          seconds: rep.seconds,
        })
      } else {
        repRow.reference ??= rep.telemetry
        rows.push(repRow)
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
          rep.repTelemetry &&
          rep.telemetry?.spanID &&
          referenceKey(rep.repTelemetry) !== referenceKey(rep.telemetry)
        ) {
          parent = {
            id: `execution/${rep.id}`,
            parentID: owner.id,
            kind: 'execution',
            caseID: group.id,
            rep,
            name: nodeFor(rep.telemetry)?.spanData.name ?? 'Execution',
            depth: owner.depth + 1,
            reference: rep.telemetry,
            window: spanWindow([nodeFor(rep.telemetry)]),
            seconds: rep.seconds,
            state: execution.state,
            internalFault: execution.internalFault,
            spanError: execution.spanError,
            estimate: estimate(rep.investigatorCost),
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
            rep,
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
      if (group.allReps.length === 1) {
        caseRow.window = repRow.window
        caseRow.seconds = repRow.seconds
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
    (row.kind === 'run'
      ? true
      : row.kind === 'case'
        ? views.includes('results') || views.includes('time')
        : row.kind === 'rep' || row.kind === 'execution'
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
