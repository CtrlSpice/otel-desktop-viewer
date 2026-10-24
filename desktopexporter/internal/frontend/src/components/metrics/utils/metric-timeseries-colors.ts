// The context owns the visible colour map. The saved, metric-scoped registry
// retains logical slots even while a series is hidden or a metric is closed.
import { identityColorSlot } from '@/utils/color-slots'

/** Checked native series identity → current theme's assigned colour. */
export type TimeseriesColorByKey = Map<string, string>

/**
 * Restore saved slots, or hash and probe for a new identity in legend order.
 * Encounter order affects new collision winners, never existing assignments.
 */
export function seedColorAssignments(
  pool: readonly string[],
  visibleKeys: ReadonlySet<string>,
  legendOrder: readonly string[],
  scope: string = 'metric-series'
): TimeseriesColorByKey {
  const out: TimeseriesColorByKey = new Map()
  for (const key of legendOrder) {
    if (!visibleKeys.has(key)) continue
    acquireColor(pool, out, key, scope)
  }
  return out
}

/** Saved hash/probe slot; after exhaustion reuse without moving any owner. */
export function acquireColor(
  pool: readonly string[],
  assigned: TimeseriesColorByKey,
  key: string,
  scope: string = 'metric-series'
): string | null {
  const existing = assigned.get(key)
  if (existing !== undefined) return existing
  if (pool.length === 0) return null
  const color = pool[identityColorSlot(key, scope, pool.length)]!
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

/** Drop hidden render entries, but keep their saved slots for later restoration. */
export function syncColorAssignments(
  pool: readonly string[],
  assigned: TimeseriesColorByKey,
  visibleKeys: ReadonlySet<string>,
  legendOrder: readonly string[],
  scope: string = 'metric-series'
): void {
  for (const key of Array.from(assigned.keys())) {
    if (!visibleKeys.has(key)) assigned.delete(key)
  }
  for (const key of legendOrder) {
    if (!visibleKeys.has(key)) continue
    acquireColor(pool, assigned, key, scope)
  }
}
