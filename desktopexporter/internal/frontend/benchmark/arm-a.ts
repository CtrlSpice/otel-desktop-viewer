import { mount, tick, unmount } from 'svelte'
import { telemetryAPI } from '@/services/telemetry-service'
import type { TraceData } from '@/types/api-types'
import ArmAHarness from './ArmAHarness.svelte'
import {
  hashTraceWaterfallProjection,
  projectTraceWaterfall,
} from './canonical-oracle'
import type {
  ArmARunContract,
  ArmARunResult,
  BigIntRehydrationProof,
  BigIntValueProof,
  TraceWaterfallBenchmarkAPI,
  ViewportGeometry,
} from './api-types'

const READINESS_TIMEOUT_MS = 15_000
const POLL_INTERVAL_MS = 16
const GRID_SELECTOR = '[role="grid"][aria-label="Span waterfall"]'
const VIEWPORT_SELECTOR = '.waterfall-vlist-viewport'
const ROW_SELECTOR = 'tr[data-span-id]'

type RenderState = {
  ariaRowCount: number | null
  viewportGeometry: ViewportGeometry
  firstVisibleSpanID: string | null
  mountedRowCount: number
}

function requireNonemptyString(value: string, field: string): void {
  if (typeof value !== 'string' || value.length === 0) {
    throw new TypeError(`${field} must be a nonempty string`)
  }
}

function requireSafeInteger(
  value: number,
  field: string,
  minimum: number
): void {
  if (!Number.isSafeInteger(value) || value < minimum) {
    throw new TypeError(`${field} must be a safe integer >= ${minimum}`)
  }
}

function validateContract(contract: ArmARunContract): void {
  if (contract === null || typeof contract !== 'object') {
    throw new TypeError('Arm A contract must be an object')
  }
  requireNonemptyString(contract.fixtureName, 'fixtureName')
  requireNonemptyString(contract.traceID, 'traceID')
  requireSafeInteger(
    contract.expectedDisplayedSpanCount,
    'expectedDisplayedSpanCount',
    1
  )
  requireSafeInteger(
    contract.expectedMaximumDisplayedDepth,
    'expectedMaximumDisplayedDepth',
    0
  )
  requireNonemptyString(contract.expectedFirstSpanID, 'expectedFirstSpanID')
}

function emptyGeometry(): ViewportGeometry {
  return { x: 0, y: 0, width: 0, height: 0 }
}

function geometryOf(element: Element): ViewportGeometry {
  const rect = element.getBoundingClientRect()
  return {
    x: rect.x,
    y: rect.y,
    width: rect.width,
    height: rect.height,
  }
}

function inspectRender(target: Element): RenderState {
  const grid = target.querySelector<HTMLElement>(GRID_SELECTOR)
  const viewport = grid?.querySelector<HTMLElement>(VIEWPORT_SELECTOR) ?? null
  const ariaRowCountValue = grid?.getAttribute('aria-rowcount') ?? null
  const ariaRowCount =
    ariaRowCountValue !== null && /^\d+$/.test(ariaRowCountValue)
      ? Number(ariaRowCountValue)
      : null
  const rows = viewport
    ? Array.from(viewport.querySelectorAll<HTMLTableRowElement>(ROW_SELECTOR))
    : []
  const viewportGeometry = viewport ? geometryOf(viewport) : emptyGeometry()
  const viewportRect = viewport?.getBoundingClientRect() ?? null
  const firstVisibleRow =
    viewportRect === null
      ? undefined
      : rows.find(row => {
          const rect = row.getBoundingClientRect()
          const style = getComputedStyle(row)
          return (
            rect.height > 0 &&
            rect.bottom > viewportRect.top &&
            rect.top < viewportRect.bottom &&
            style.display !== 'none' &&
            style.visibility === 'visible'
          )
        })

  return {
    ariaRowCount,
    viewportGeometry,
    firstVisibleSpanID: firstVisibleRow?.dataset.spanId ?? null,
    mountedRowCount: rows.length,
  }
}

function isReady(state: RenderState, contract: ArmARunContract): boolean {
  return (
    state.ariaRowCount === contract.expectedDisplayedSpanCount &&
    state.viewportGeometry.width > 0 &&
    state.viewportGeometry.height > 0 &&
    state.firstVisibleSpanID === contract.expectedFirstSpanID
  )
}

function readinessDescription(
  state: RenderState,
  contract: ArmARunContract
): string {
  return (
    `expected aria-rowcount=${contract.expectedDisplayedSpanCount}, ` +
    `first visible span=${contract.expectedFirstSpanID}, and nonzero viewport; ` +
    `observed aria-rowcount=${state.ariaRowCount ?? 'missing'}, ` +
    `first visible span=${state.firstVisibleSpanID ?? 'missing'}, ` +
    `viewport=${state.viewportGeometry.width}x${state.viewportGeometry.height}, ` +
    `mounted rows=${state.mountedRowCount}`
  )
}

function delay(milliseconds: number): Promise<void> {
  return new Promise(resolve => window.setTimeout(resolve, milliseconds))
}

async function waitForReady(
  target: Element,
  contract: ArmARunContract
): Promise<RenderState> {
  const deadline = performance.now() + READINESS_TIMEOUT_MS
  let state = inspectRender(target)

  while (!isReady(state, contract) && performance.now() < deadline) {
    await delay(POLL_INTERVAL_MS)
    state = inspectRender(target)
  }

  if (!isReady(state, contract)) {
    throw new Error(
      `Arm A fixture "${contract.fixtureName}" did not stabilize within ` +
        `${READINESS_TIMEOUT_MS}ms: ${readinessDescription(state, contract)}`
    )
  }
  return state
}

async function withTimeout<T>(
  promise: Promise<T>,
  milliseconds: number,
  message: string
): Promise<T> {
  let timeoutID: number | undefined
  const timeout = new Promise<never>((_resolve, reject) => {
    timeoutID = window.setTimeout(
      () => reject(new Error(message)),
      milliseconds
    )
  })
  try {
    return await Promise.race([promise, timeout])
  } finally {
    if (timeoutID !== undefined) window.clearTimeout(timeoutID)
  }
}

function nextAnimationFrame(): Promise<void> {
  return new Promise(resolve => requestAnimationFrame(() => resolve()))
}

function bigintValueProof(value: unknown): BigIntValueProof {
  return { type: typeof value, decimal: String(value) }
}

function proveBigIntRehydration(trace: TraceData): BigIntRehydrationProof {
  const firstSpan = trace.spans[0]?.spanData ?? null
  const firstEvent =
    trace.spans.find(node => node.spanData.events.length > 0)?.spanData
      .events[0] ?? null

  return {
    allSpanStartAndEndTimesAreBigInts: trace.spans.every(
      node =>
        typeof node.spanData.startTime === 'bigint' &&
        typeof node.spanData.endTime === 'bigint'
    ),
    allEventTimestampsAreBigInts: trace.spans.every(node =>
      node.spanData.events.every(event => typeof event.timestamp === 'bigint')
    ),
    sampleExceedsNumberSafeInteger:
      typeof firstSpan?.startTime === 'bigint' &&
      firstSpan.startTime > BigInt(Number.MAX_SAFE_INTEGER),
    firstSpanStartTime: firstSpan
      ? bigintValueProof(firstSpan.startTime)
      : null,
    firstSpanEndTime: firstSpan ? bigintValueProof(firstSpan.endTime) : null,
    firstEventTimestamp: firstEvent
      ? bigintValueProof(firstEvent.timestamp)
      : null,
  }
}

function assertProjectionMatchesContract(
  result: Pick<
    ArmARunResult,
    | 'traceID'
    | 'displayedSpanCount'
    | 'maximumDisplayedDepth'
    | 'firstDisplayedSpanID'
  >,
  contract: ArmARunContract
): void {
  const mismatches: string[] = []
  if (result.traceID !== contract.traceID) {
    mismatches.push(`traceID=${result.traceID}`)
  }
  if (result.displayedSpanCount !== contract.expectedDisplayedSpanCount) {
    mismatches.push(`displayedSpanCount=${result.displayedSpanCount}`)
  }
  if (result.maximumDisplayedDepth !== contract.expectedMaximumDisplayedDepth) {
    mismatches.push(`maximumDisplayedDepth=${result.maximumDisplayedDepth}`)
  }
  if (result.firstDisplayedSpanID !== contract.expectedFirstSpanID) {
    mismatches.push(`firstDisplayedSpanID=${result.firstDisplayedSpanID}`)
  }
  if (mismatches.length > 0) {
    throw new Error(
      `Arm A fixture "${contract.fixtureName}" violated its contract: ` +
        mismatches.join(', ')
    )
  }
}

export function createArmABenchmarkAPI(
  target: HTMLElement
): TraceWaterfallBenchmarkAPI {
  let activeHarness: Record<string, unknown> | null = null
  let runInProgress = false

  async function clearPreviousRun(): Promise<void> {
    if (activeHarness !== null) {
      const previousHarness = activeHarness
      activeHarness = null
      await unmount(previousHarness)
    }
    target.replaceChildren()
  }

  async function runArmA(contract: ArmARunContract): Promise<ArmARunResult> {
    validateContract(contract)
    if (runInProgress) {
      throw new Error('Arm A does not permit concurrent runs')
    }
    runInProgress = true

    try {
      await clearPreviousRun()

      // Calling with only traceID is the production no-search path. Its private
      // traceDataFromJSON reviver remains the sole owner of wire rehydration.
      const trace = await telemetryAPI.searchSpans(contract.traceID)

      activeHarness = mount(ArmAHarness, {
        target,
        props: { trace },
      })

      await tick()
      await withTimeout(
        document.fonts.ready,
        READINESS_TIMEOUT_MS,
        `Arm A fixture "${contract.fixtureName}" fonts did not become ready ` +
          `within ${READINESS_TIMEOUT_MS}ms`
      )
      await waitForReady(target, contract)

      await nextAnimationFrame()
      await nextAnimationFrame()

      const stableRender = inspectRender(target)
      if (!isReady(stableRender, contract)) {
        throw new Error(
          `Arm A fixture "${contract.fixtureName}" became unstable after two ` +
            `animation frames: ${readinessDescription(stableRender, contract)}`
        )
      }

      // The Phase 2 oracle is deliberately outside the stable-render gate.
      const projection = projectTraceWaterfall(trace)
      const semanticHash = await hashTraceWaterfallProjection(projection)
      const maximumDisplayedDepth = projection.spans.reduce(
        (maximum, span) => Math.max(maximum, span.depth),
        0
      )
      const displayRootSpanIDs = projection.spans
        .filter(span => span.depth === 0)
        .map(span => span.spanID)

      const result: ArmARunResult = {
        arm: 'A',
        fixtureName: contract.fixtureName,
        traceID: projection.traceID,
        displayedSpanCount: projection.spans.length,
        maximumDisplayedDepth,
        firstDisplayedSpanID: projection.spans[0]?.spanID ?? null,
        displayRootSpanIDs,
        unplacedSpanCount: projection.unplacedSpanCount,
        viewportGeometry: stableRender.viewportGeometry,
        mountedRowCount: stableRender.mountedRowCount,
        bigintRehydration: proveBigIntRehydration(trace),
        semanticHash,
        topology: projection.spans.map(span => ({
          spanID: span.spanID,
          parentSpanID: span.parentSpanID,
          depth: span.depth,
          salvaged: span.salvaged,
          cyclePoint: span.cyclePoint,
        })),
      }

      assertProjectionMatchesContract(result, contract)
      JSON.stringify(result)
      return result
    } finally {
      runInProgress = false
    }
  }

  return { runArmA }
}
