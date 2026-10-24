import { GRAPH_SWATCHES, graphSwatch } from '@/themes/graph-inks'
import { graphColorOverrides } from '@/state/graph-color-overrides.svelte'
import { savedColorSlots } from './color-slots'

export function graphColorAvailability(scope: string, identity: string) {
  const taken = new Set<string>()
  const unavailable = new Set<string>()
  let selected = graphColorOverrides.get(scope, identity)
  for (const [owner, slot] of savedColorSlots(scope)) {
    const swatch = GRAPH_SWATCHES[slot]!
    taken.add(swatch.color)
    if (owner !== identity) unavailable.add(swatch.color)
    else selected ??= swatch
  }
  for (const [owner, id] of graphColorOverrides.entries(scope)) {
    const swatch = graphSwatch(id)!
    taken.add(swatch.color)
    if (owner !== identity) unavailable.add(swatch.color)
  }
  // Preserve an existing choice even if older storage already has a collision.
  if (selected) unavailable.delete(selected.color)
  return {
    selected,
    unavailable,
    freePresets: GRAPH_SWATCHES.filter(swatch => !taken.has(swatch.color))
      .length,
  }
}
