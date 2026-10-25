import {
  GRAPH_PALETTE_VERSION,
  GRAPH_SWATCHES,
  graphSwatch,
  type GraphColorId,
} from '@/themes/graph-inks'

const PREFIX = `graph-colour-overrides:${GRAPH_PALETTE_VERSION}:`
type ColorOverrideId = GraphColorId | `#${string}`
const scopes = new Map<string, Map<string, ColorOverrideId>>()
// Allocate the reactive signal at module scope: a scope first read inside a
// derived value must still notify that first reader when an override changes.
let revision = $state(0)

function isSavedOverride(value: unknown): value is [string, string] {
  return (
    Array.isArray(value) &&
    value.length === 2 &&
    typeof value[0] === 'string' &&
    typeof value[1] === 'string'
  )
}

function resolveOverride(value: string | undefined) {
  if (value && /^#[0-9a-f]{6}$/i.test(value)) {
    // SAFETY: the regex requires a leading # and exactly six hexadecimal digits.
    const color = value.toLowerCase() as `#${string}`
    return { id: color, color }
  }
  return graphSwatch(value)
}

function readSaved(scope: string): [string, ColorOverrideId][] {
  try {
    const raw: unknown = JSON.parse(
      localStorage.getItem(PREFIX + scope) ?? '[]'
    )
    if (!Array.isArray(raw)) return []
    return raw
      .filter(isSavedOverride)
      .flatMap((entry): [string, ColorOverrideId][] => {
        // Accept native picker hex values and existing saved ink/shade IDs.
        const swatch = resolveOverride(entry[1])
        return swatch ? [[entry[0], swatch.id]] : []
      })
  } catch {
    return []
  }
}

function forScope(scope: string): Map<string, ColorOverrideId> {
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
    return resolveOverride(forScope(scope).get(identity))
  },
  entries(scope: string): [string, ColorOverrideId][] {
    void revision
    return [...forScope(scope)]
  },
  set(scope: string, identity: string, value: string | null) {
    const resolved = value === null ? null : resolveOverride(value)
    if (value !== null && !resolved) return
    const values = forScope(scope)
    for (const [key, id] of readSaved(scope)) {
      if (!values.has(key)) values.set(key, id)
    }
    if (value === null) values.delete(identity)
    else if (resolved) values.set(identity, resolved.id)
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
      [...forScope(scope).values()].map(id => resolveOverride(id)!.color)
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
