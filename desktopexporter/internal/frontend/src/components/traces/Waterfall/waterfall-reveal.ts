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

/** Remove collapsed markers on every ancestor of spanId. Mutates collapsed in place. */
export function expandAncestorsForSpan(
  collapsed: Set<string>,
  spanId: string,
  parentOf: ReadonlyMap<string, string | null>
): boolean {
  let changed = false
  for (const id of ancestorIdsOf(spanId, parentOf)) {
    if (collapsed.delete(id)) changed = true
  }
  return changed
}
