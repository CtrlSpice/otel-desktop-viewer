// Colour-slot assignment for metric timeseries. Given a stem-rotated palette
// (from `categoricalPalette`) and the current visible/legend sets, these pure
// helpers decide which key gets which colour. They hold no reactive state —
// metric-view-context owns the `$state` map and calls these to mutate a copy.

/** Checked timeseries key → assigned colour from the rotated pool. */
export type TimeseriesColorByKey = Map<string, string>

/**
 * Assign colours from a stem-rotated pool (from `categoricalPalette`)
 * to an initial visible set. Walks `legendOrder` so slot-filling
 * follows list order; the first assigned key gets `pool[0]`, the requested
 * family’s approved starting swatch.
 */
export function seedColorAssignments(
  pool: readonly string[],
  visibleKeys: ReadonlySet<string>,
  legendOrder: readonly string[]
): TimeseriesColorByKey {
  const out: TimeseriesColorByKey = new Map()
  let i = 0
  for (const key of legendOrder) {
    if (!visibleKeys.has(key)) continue
    if (i >= pool.length) break
    out.set(key, pool[i++]!)
  }
  return out
}

/** First unused colour in `pool` order, then deterministic reuse when finite. */
export function acquireColor(
  pool: readonly string[],
  assigned: TimeseriesColorByKey,
  key: string
): string | null {
  const existing = assigned.get(key)
  if (existing !== undefined) return existing
  const used = new Set(assigned.values())
  for (const color of pool) {
    if (!used.has(color)) {
      assigned.set(key, color)
      return color
    }
  }
  if (pool.length === 0) return null
  const color = pool[assigned.size % pool.length]!
  assigned.set(key, color)
  return color
}

/** Remap assigned keys to corresponding slots in a new theme's palette. */
export function remapColorAssignments(
  assigned: TimeseriesColorByKey,
  previousPool: readonly string[],
  nextPool: readonly string[]
): TimeseriesColorByKey {
  if (nextPool.length === 0) return new Map()
  return new Map(
    [...assigned].map(([key, color]) => {
      const slot = previousPool.indexOf(color)
      return [key, nextPool[(slot < 0 ? 0 : slot) % nextPool.length]!]
    })
  )
}

export function releaseColor(
  assigned: TimeseriesColorByKey,
  key: string
): void {
  assigned.delete(key)
}

/** Drop unchecked keys; acquire for newly visible keys in legend order. */
export function syncColorAssignments(
  pool: readonly string[],
  assigned: TimeseriesColorByKey,
  visibleKeys: ReadonlySet<string>,
  legendOrder: readonly string[]
): void {
  for (const key of Array.from(assigned.keys())) {
    if (!visibleKeys.has(key)) assigned.delete(key)
  }
  for (const key of legendOrder) {
    if (!visibleKeys.has(key)) continue
    acquireColor(pool, assigned, key)
  }
}
