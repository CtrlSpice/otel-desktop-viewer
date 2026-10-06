import { SvelteMap } from 'svelte/reactivity'

/**
 * Session-scoped collapse state keyed by trace ID. Module scope preserves it
 * across component remounts, and insertion-order eviction bounds memory use.
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
