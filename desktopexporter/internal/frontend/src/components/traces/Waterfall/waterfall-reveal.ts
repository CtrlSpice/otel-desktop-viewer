/** Walk parentSpanID links from spanId up to the trace root.
 *
 * Carries a visited set because parentSpanID is reported data, not verified
 * structure: a trace with a parent cycle (see `salvaged`/`cyclePoint`) makes
 * two spans each other's ancestor, and an unguarded walk here hangs the main
 * thread the moment a ?span= link targets a cycle member. The walk stops at
 * the first repeat -- same rule the server's tree walk applies in SQL.
 */
export function ancestorIdsOf(
  spanId: string,
  parentOf: ReadonlyMap<string, string | null>
): string[] {
  const ancestors: string[] = []
  const seen = new Set([spanId])
  let pid = parentOf.get(spanId) ?? null
  while (pid !== null && !seen.has(pid)) {
    seen.add(pid)
    ancestors.push(pid)
    pid = parentOf.get(pid) ?? null
  }
  return ancestors
}
