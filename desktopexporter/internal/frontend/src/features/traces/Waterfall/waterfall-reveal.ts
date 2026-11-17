/** Walk parentSpanID links from spanID up to the trace root.
 *
 * Carries a visited set because parentSpanID is reported data, not verified
 * structure: a trace with a parent cycle (see `salvaged`/`cyclePoint`) makes
 * two spans each other's ancestor, and an unguarded walk here hangs the main
 * thread the moment a ?span= link targets a cycle member. The walk stops at
 * the first repeat -- same rule the server's tree walk applies in SQL.
 */
export function ancestorIdsOf(
  spanID: string,
  parentOf: ReadonlyMap<string, string | null>
): string[] {
  const ancestors: string[] = []
  const seen = new Set([spanID])
  let pid = parentOf.get(spanID) ?? null
  while (pid !== null && !seen.has(pid)) {
    seen.add(pid)
    ancestors.push(pid)
    pid = parentOf.get(pid) ?? null
  }
  return ancestors
}

/** Choose the single visible row through which keyboard focus enters the tree. */
export function keyboardAnchorSpanID(
  selectedSpanID: string | null,
  visibleSpanIDs: readonly string[],
  parentOf: ReadonlyMap<string, string | null>
): string | null {
  const firstVisibleID = visibleSpanIDs[0] ?? null
  if (!selectedSpanID) return firstVisibleID

  const visible = new Set(visibleSpanIDs)
  if (visible.has(selectedSpanID)) return selectedSpanID

  return (
    ancestorIdsOf(selectedSpanID, parentOf).find(id => visible.has(id)) ??
    firstVisibleID
  )
}
