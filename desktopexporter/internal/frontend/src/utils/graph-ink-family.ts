import { GRAPH_INKS, graphSwatch } from '@/themes/graph-inks'
import {
  linearToOklab,
  srgb8ToLinear,
} from '../../../../../third_party/madoromi/color'

function lab(hex: string) {
  return linearToOklab(
    srgb8ToLinear(parseInt(hex.slice(1, 3), 16)),
    srgb8ToLinear(parseInt(hex.slice(3, 5), 16)),
    srgb8ToLinear(parseInt(hex.slice(5, 7), 16))
  )
}

const candidates = GRAPH_INKS.flatMap(ink =>
  ink.swatches.map(swatch => ({
    ink,
    color: swatch.color,
    lab: lab(swatch.color),
  }))
)

/** Compare uncomposited RGB in OKLab: opacity never changes the ink family. */
export function graphInkFamily(color: string, savedID?: string) {
  const known = graphSwatch(savedID)
  if (known)
    return { ink: GRAPH_INKS.find(ink => ink.id === known.ink)!, exact: true }
  const rgb = color.slice(0, 7).toLowerCase()
  const exact = candidates.find(candidate => candidate.color === rgb)
  if (exact) return { ink: exact.ink, exact: true }
  const target = lab(rgb)
  let nearest = candidates[0]!
  let distance = Infinity
  for (const candidate of candidates) {
    const next = candidate.lab.reduce(
      (sum, value, index) => sum + (value - target[index]!) ** 2,
      0
    )
    if (next < distance) {
      nearest = candidate
      distance = next
    }
  }
  return { ink: nearest.ink, exact: false }
}
