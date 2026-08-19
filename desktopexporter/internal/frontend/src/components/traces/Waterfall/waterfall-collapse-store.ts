import { SvelteMap } from 'svelte/reactivity'

/**
 * The reader's collapse set, per trace, held at module level.
 *
 * Module level because component state does not survive the component.
 * WaterfallView lives in an `{:else if}` chain on TracesPage, so a refetch
 * passing through a loading state, a switch away and back, or a layout change
 * that moves the pane all recreate it -- and a recreated component with local
 * collapse state comes back fully expanded, indistinguishable from the view
 * "collapsing itself" (or, before the auto-collapse heuristic was removed,
 * re-collapsing itself; that was #348). Holding the set out here makes remount
 * a non-event.
 *
 * Keyed by trace ID: a different trace is a different tree, so it gets fresh
 * state, while returning to a trace finds it as it was left. Session-scoped on
 * purpose -- a page reload starts expanded, which is the contract elsewhere in
 * the app (legend selections persist, chart tabs persist, but tree shape is
 * per-visit).
 *
 * Bounded: a long session browsing many traces must not grow this forever.
 * Insertion order is the eviction order; revisiting an evicted trace just
 * means it opens expanded again.
 */
const MAX_TRACKED_TRACES = 20

const collapsedByTrace = new SvelteMap<string, ReadonlySet<string>>()

const EMPTY: ReadonlySet<string> = new Set()

export function collapsedForTrace(traceID: string): ReadonlySet<string> {
  return collapsedByTrace.get(traceID) ?? EMPTY
}

export function setCollapsedForTrace(
  traceID: string,
  next: ReadonlySet<string>
): void {
  if (!collapsedByTrace.has(traceID)) {
    while (collapsedByTrace.size >= MAX_TRACKED_TRACES) {
      const oldest = collapsedByTrace.keys().next().value
      if (oldest === undefined) break
      collapsedByTrace.delete(oldest)
    }
  }
  collapsedByTrace.set(traceID, next)
}

/** Tests share module state; each starts from an empty store. */
export function resetCollapseStoreForTests(): void {
  collapsedByTrace.clear()
}
