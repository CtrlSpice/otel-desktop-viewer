import harmonies from './service-ink-harmonies.json' with { type: 'json' }
import type { AccentInkId } from './accent-inks'
import { GRAPH_INKS, GRAPH_SWATCHES, type GraphShade } from './graph-inks'

const slots = new Map<string, number>(
  GRAPH_SWATCHES.map((swatch, slot) => [swatch.id, slot])
)
const cache = new Map<AccentInkId, readonly number[]>()

/** Analogous core, early opposite-hue accents, then the entire official catalogue. */
export function serviceInkSlots(seed: AccentInkId): readonly number[] {
  const cached = cache.get(seed)
  if (cached) return cached
  const family = harmonies.inks[seed]
  const preferred = new Set([...family.analogous, ...family.complementary])
  const broader = new Set(
    [...family.triadic, ...family.tetradic].filter(id => !preferred.has(id))
  )
  const remaining = GRAPH_INKS.map(ink => ink.id).filter(
    id => !preferred.has(id) && !broader.has(id)
  )
  const ranked = [...preferred, ...broader, ...remaining]
  const interleave = (inks: readonly string[], shades: readonly GraphShade[]) =>
    shades.flatMap(shade =>
      inks.map(ink => {
        const slot = slots.get(`${ink}:${shade}`)
        if (slot === undefined)
          throw new Error(`Unknown service ink shade: ${ink}:${shade}`)
        return slot
      })
    )
  const order = [
    ...interleave([...preferred], [4, 2, 6]),
    ...interleave([...broader], [4, 2, 6]),
    ...interleave(remaining, [4, 2, 6]),
    ...interleave(ranked, [1, 3, 5, 7]),
  ]
  cache.set(seed, order)
  return order
}
