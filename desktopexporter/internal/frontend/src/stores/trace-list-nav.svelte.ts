/** Shared sorted trace IDs for prev/next between list and detail (module survives route changes). */
let ids = $state<string[]>([])

export function getTraceListNavIds(): readonly string[] {
  return ids
}

export function setTraceListNavIds(next: readonly string[]) {
  ids = next.length ? [...next] : []
}
