// Per-signal monotonic sequence for list updates (full fetch vs search results).
// Each operation calls beginListUpdate(signal) when it *starts*; completions
// apply only if their seq is still the latest for that signal.

import type { SignalName } from '@/route'

const latestListUpdateSeq = new Map<SignalName, number>()

function latestSeq(signal: SignalName): number {
  return latestListUpdateSeq.get(signal) ?? 0
}

/** Call when a list fetch or search submit begins. Returns this operation's seq. */
export function beginListUpdate(signal: SignalName): number {
  const next = latestSeq(signal) + 1
  latestListUpdateSeq.set(signal, next)
  return next
}

/** True when no newer list update has started for `signal` since `seq` was issued. */
export function isLatestListUpdate(signal: SignalName, seq: number): boolean {
  return seq === latestSeq(signal)
}

/** Invalidate in-flight list fetches/searches for `signal` (e.g. on page unmount). */
export function cancelPendingListUpdates(signal: SignalName): void {
  beginListUpdate(signal)
}

/** Test-only reset. */
export function resetListUpdateSeqForTests(): void {
  latestListUpdateSeq.clear()
}
