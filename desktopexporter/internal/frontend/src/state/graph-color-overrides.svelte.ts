import {
  GRAPH_PALETTE_VERSION,
  GRAPH_SWATCHES,
  graphSwatch,
  type GraphColorId,
} from '@/themes/graph-inks'

const PREFIX = `graph-colour-overrides:${GRAPH_PALETTE_VERSION}:`
const scopes = new Map<string, Map<string, GraphColorId>>()
// Allocate the reactive signal at module scope: a scope first read inside a
// derived value must still notify that first reader when an override changes.
let revision = $state(0)

function readSaved(scope: string): [string, GraphColorId][] {
  try {
    const raw: unknown = JSON.parse(
      localStorage.getItem(PREFIX + scope) ?? '[]'
    )
    if (!Array.isArray(raw)) return []
    return raw.flatMap(entry => {
      // Browser storage is an external boundary; accept known ink/shade IDs only.
      if (
        !Array.isArray(entry) ||
        entry.length !== 2 ||
        typeof entry[0] !== 'string' ||
        typeof entry[1] !== 'string'
      )
        return []
      const swatch = graphSwatch(entry[1])
      return swatch ? [[entry[0], swatch.id] as [string, GraphColorId]] : []
    })
  } catch {
    return []
  }
}

function forScope(scope: string): Map<string, GraphColorId> {
  let values = scopes.get(scope)
  if (!values) {
    values = new Map(readSaved(scope))
    scopes.set(scope, values)
  }
  return values
}

export const graphColorOverrides = {
  get(scope: string, identity: string) {
    void revision
    return graphSwatch(forScope(scope).get(identity))
  },
  entries(scope: string): [string, GraphColorId][] {
    void revision
    return [...forScope(scope)]
  },
  set(scope: string, identity: string, value: GraphColorId | null) {
    const values = forScope(scope)
    for (const [key, id] of readSaved(scope)) {
      if (!values.has(key)) values.set(key, id)
    }
    if (value === null) values.delete(identity)
    else values.set(identity, value)
    revision++
    try {
      localStorage.setItem(PREFIX + scope, JSON.stringify([...values]))
    } catch {
      // Retain the override for this page when storage is full or blocked.
    }
  },
  reservedSlots(scope: string): ReadonlySet<number> {
    void revision
    const selected = new Set(
      [...forScope(scope).values()].map(id => graphSwatch(id)!.color)
    )
    return new Set(
      GRAPH_SWATCHES.flatMap((swatch, slot) =>
        selected.has(swatch.color) ? [slot] : []
      )
    )
  },
}

if (typeof window !== 'undefined') {
  window.addEventListener('storage', event => {
    if (event.key !== null && !event.key.startsWith(PREFIX)) return
    for (const [scope, values] of scopes) {
      if (event.key !== null && event.key !== PREFIX + scope) continue
      const saved = readSaved(scope)
      values.clear()
      for (const [identity, id] of saved) values.set(identity, id)
    }
    revision++
  })
}
