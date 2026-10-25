import rankings from './bright-ink-slots.json' with { type: 'json' }
import type { AccentInkId } from './accent-inks'
import { GRAPH_SWATCHES } from './graph-inks'

export const AUTOMATIC_COLOR_VERSION = rankings.version

const slots = new Map<string, number>(
  GRAPH_SWATCHES.map((swatch, slot) => [swatch.id, slot])
)
const cache = new Map<AccentInkId, readonly number[]>()

/** Three related + two triadic, then other bright official swatches. */
export function serviceInkSlots(seed: AccentInkId): readonly number[] {
  const cached = cache.get(seed)
  if (cached) return cached
  const order = rankings.inks[seed].map(id => {
    const slot = slots.get(id)
    if (slot === undefined)
      throw new Error(`Unknown automatic ink shade: ${id}`)
    return slot
  })
  cache.set(seed, order)
  return order
}
