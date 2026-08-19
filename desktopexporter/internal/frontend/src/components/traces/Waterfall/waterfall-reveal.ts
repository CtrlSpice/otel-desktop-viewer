/** Walk parentSpanID links from spanId up to the trace root. */
export function ancestorIdsOf(
  spanId: string,
  parentOf: ReadonlyMap<string, string | null>
): string[] {
  const ancestors: string[] = []
  let pid = parentOf.get(spanId) ?? null
  while (pid !== null) {
    ancestors.push(pid)
    pid = parentOf.get(pid) ?? null
  }
  return ancestors
}
