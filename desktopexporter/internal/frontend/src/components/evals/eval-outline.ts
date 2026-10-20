import type { SpanNode } from '@/types/api-types'
import {
  computeTreeMeta,
  isErrorSpan,
} from '@/components/traces/Waterfall/WaterfallView.svelte'
import { completionState, type groupAttempts } from './group-attempts'
import {
  estimatedCost,
  type EvalAttempt,
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
  kind: 'case' | 'attempt' | 'execution' | 'grading' | 'tool' | 'grader'
  caseID: string
  attempt?: EvalAttempt
  name: string
  depth: number
  state: EvalAttempt['state']
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
  let window: TimeWindow | undefined
  for (const node of nodes) {
    const span = node?.spanData
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
  groups: ReturnType<typeof groupAttempts>,
  run: EvalRun,
  index: ReturnType<typeof indexEvalTraces>
): EvalOutlineRow[] {
  const source = new Map(run.attempts.map(attempt => [attempt.id, attempt]))
  const nodeFor = (reference?: TelemetryReference) =>
    reference ? index.nodes.get(referenceKey(reference)) : undefined
  const rows: EvalOutlineRow[] = []
  const emptyTree = {
    childrenCount: 0,
    ancestorHasNextSibling: [],
    isLastChild: true,
  }
  for (const group of groups) {
    const caseRow: EvalOutlineRow = {
      id: `case/${group.id}`,
      parentID: null,
      kind: 'case',
      caseID: group.id,
      name: group.name,
      depth: 0,
      state: group.state,
      internalFault: false,
      spanError: false,
      estimate: combineEstimates([
        estimatedCost(group.allAttempts, 'investigatorCost'),
        estimatedCost(group.allAttempts, 'graderCost'),
      ]),
      tree: emptyTree,
    }
    rows.push(caseRow)
    // Hidden repetitions still contribute to the case's status and cost.
    const statuses = group.allAttempts.map(item => {
      const attempt = source.get(item.id)!
      const execution = evalExecutionRows(attempt, index)
      const attemptNode = nodeFor(attempt.attemptTelemetry)
      const spanError =
        execution.spanError ||
        !!(attemptNode && isErrorSpan(attemptNode.spanData))
      return {
        state: spanError ? ('Error' as const) : execution.state,
        spanError,
        internalFault: execution.internalFault,
      }
    })
    caseRow.state = completionState(statuses)
    caseRow.spanError = statuses.some(item => item.spanError)
    caseRow.internalFault = statuses.some(item => item.internalFault)
    for (const item of group.attempts) {
      const attempt = source.get(item.id)!
      const execution = evalExecutionRows(
        { ...attempt, assessments: [] },
        index
      )
      const grading = evalExecutionRows(
        { ...attempt, telemetry: undefined, tools: [], state: 'Complete' },
        index
      )
      const attemptNode = nodeFor(attempt.attemptTelemetry)
      const spanError =
        execution.spanError ||
        grading.spanError ||
        !!(attemptNode && isErrorSpan(attemptNode.spanData))
      const attemptRow: EvalOutlineRow = {
        id: `attempt/${attempt.id}`,
        parentID: caseRow.id,
        kind: 'attempt',
        caseID: group.id,
        attempt,
        name: `Attempt ${attempt.repetition}`,
        depth: 1,
        state: spanError ? 'Error' : attempt.state,
        internalFault: attempt.state === 'Error',
        spanError,
        reference: attempt.attemptTelemetry,
        window:
          spanWindow([attemptNode]) ??
          spanWindow([
            nodeFor(attempt.telemetry),
            ...execution.rows.map(row => row.node),
            ...grading.rows.map(row => row.node),
          ]),
        seconds: attempt.seconds,
        estimate: combineEstimates([
          estimate(attempt.investigatorCost),
          estimate(attempt.graderCost),
        ]),
        tree: emptyTree,
      }
      rows.push(attemptRow)
      const phases = [
        {
          kind: 'execution' as const,
          name: 'Execution',
          reference: attempt.telemetry,
          result: execution,
          cost: attempt.investigatorCost,
        },
        ...(attempt.assessments.length || attempt.graderCost !== undefined
          ? [
              {
                kind: 'grading' as const,
                name: 'Grading',
                reference: undefined,
                result: grading,
                cost: attempt.graderCost,
              },
            ]
          : []),
      ]
      for (const phase of phases) {
        const phaseRow: EvalOutlineRow = {
          id: `${phase.kind}/${attempt.id}`,
          parentID: attemptRow.id,
          kind: phase.kind,
          caseID: group.id,
          attempt,
          name: phase.name,
          depth: 2,
          reference: phase.reference,
          window:
            phase.kind === 'grading'
              ? spanWindow(phase.result.rows.map(row => row.node))
              : undefined,
          seconds: phase.kind === 'execution' ? attempt.seconds : undefined,
          state: phase.result.state,
          internalFault:
            phase.kind === 'execution' && attempt.state === 'Error',
          spanError: phase.result.spanError,
          estimate: estimate(phase.cost),
          tree: emptyTree,
        }
        rows.push(phaseRow)
        const parents = [phaseRow.id]
        for (const child of phase.result.rows) {
          const parentID = parents[child.depth - 1]!
          parents[child.depth] = child.id
          parents.length = child.depth + 1
          rows.push({
            id: child.id,
            parentID,
            kind: phase.kind === 'execution' ? 'tool' : 'grader',
            caseID: group.id,
            attempt,
            name: child.name,
            depth: child.depth + 2,
            state: child.state,
            internalFault: false,
            spanError: child.state === 'Error',
            reference: child.reference,
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
      if (group.allAttempts.length === 1) {
        caseRow.window = attemptRow.window
        caseRow.seconds = attemptRow.seconds
      }
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
    (row.kind === 'case'
      ? views.includes('results') || views.includes('time')
      : row.kind === 'attempt' || row.kind === 'execution'
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
