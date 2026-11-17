import ranking from './bright-ink-slots.json' with { type: 'json' }
import { GRAPH_SWATCHES } from './graph-inks'

export const AUTOMATIC_COLOR_VERSION = ranking.version
const slots = new Map<string, number>(
  GRAPH_SWATCHES.map((swatch, slot) => [swatch.id, slot])
)

/** One shared bright sequence, independent of season and Day/Night. */
export const AUTOMATIC_COLOR_SLOTS: readonly number[] = ranking.slots.map(
  id => {
    const slot = slots.get(id)
    if (slot === undefined)
      throw new Error(`Unknown automatic ink shade: ${id}`)
    return slot
  }
)
