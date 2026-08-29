import { startDrag, type DragHandle } from './drag'

/**
 * Column layout for the waterfall: widths keyed by column id, a cascade
 * for drags, and reconciliation for a column set that changes.
 *
 * @remarks
 * Everything here works in ids, never positions. The column set is about
 * to become user-configurable, and a position is a fact about one
 * particular set: add a column before another and every index shifts,
 * every stored width lands on the wrong column. An id survives.
 *
 * The engine owns no state and does not know which columns exist -- it
 * lays out whatever spec list it is handed. Which columns are in that
 * list is the caller's (eventually, a setting's) business; nothing here
 * can tell "the user hid this column" from "it never existed", which is
 * what keeps that setting cheap to build.
 *
 * Drags cascade rather than absorb. The old behaviour took space only
 * from the immediate neighbour and stopped dead when it hit its minimum,
 * with slack sitting unused two columns over. Here the shrink side gives
 * space nearest-first until the drag is satisfied or everyone is at
 * their minimum; the grow side is always the single column nearest the
 * bar. Total width is conserved by construction: exactly what is taken
 * is given.
 */

export type ColumnSpec = {
  id: string
  min: number
  /** 0 marks a fixed column: it never grows, shrinks, or hosts a bar. */
  flex: number
}

/** Pixel widths by column id -- the persisted shape, and the working one. */
export type ColumnWidths = Record<string, number>

export function fixed(id: string, width: number): ColumnSpec {
  return { id, min: width, flex: 0 }
}

export function flex(id: string, min: number, weight: number): ColumnSpec {
  return { id, min, flex: weight }
}

function totalOf(specs: ColumnSpec[], widths: ColumnWidths): number {
  return specs.reduce((s, d) => s + (widths[d.id] ?? 0), 0)
}

/**
 * Fresh layout: fixed columns take their width, flex columns share what
 * remains by weight, floored at their min.
 */
export function initialWidths(
  specs: ColumnSpec[],
  containerPx: number
): ColumnWidths {
  const fixedTotal = specs.reduce((s, d) => (d.flex === 0 ? s + d.min : s), 0)
  const weightTotal = specs.reduce((s, d) => s + d.flex, 0)
  const available = Math.max(0, containerPx - fixedTotal)

  const out: ColumnWidths = {}
  for (const d of specs) {
    out[d.id] =
      d.flex === 0
        ? d.min
        : Math.max(
            d.min,
            weightTotal > 0 ? (d.flex / weightTotal) * available : 0
          )
  }
  return out
}

/**
 * Renormalize a layout to a new container width. Growth is spread across
 * flex columns in proportion to their current widths, so the layout
 * keeps its shape; shrinkage is spread in proportion to each column's
 * slack above its min, which respects every floor and lands exactly on
 * the target whenever the target is reachable at all.
 */
export function fitWidths(
  specs: ColumnSpec[],
  current: ColumnWidths,
  containerPx: number
): ColumnWidths {
  const next: ColumnWidths = {}
  for (const d of specs) next[d.id] = current[d.id] ?? d.min

  const delta = containerPx - totalOf(specs, next)
  if (Math.abs(delta) < 0.5) return next

  const flexSpecs = specs.filter(d => d.flex > 0)
  if (flexSpecs.length === 0) return next

  if (delta > 0) {
    const widthTotal = flexSpecs.reduce((s, d) => s + next[d.id], 0)
    for (const d of flexSpecs) {
      next[d.id] +=
        widthTotal > 0
          ? (next[d.id] / widthTotal) * delta
          : delta / flexSpecs.length
    }
    return next
  }

  const need = -delta
  const slackTotal = flexSpecs.reduce(
    (s, d) => s + Math.max(0, next[d.id] - d.min),
    0
  )
  if (slackTotal <= 0) return next
  const take = Math.min(need, slackTotal)
  for (const d of flexSpecs) {
    const slack = Math.max(0, next[d.id] - d.min)
    next[d.id] -= (slack / slackTotal) * take
  }
  return next
}

/**
 * Merge a stored layout with the live column set: surviving ids keep
 * their stored widths (clamped to their min), stale ids vanish, new ids
 * get the share a fresh layout would give them, and the result is
 * renormalized to the container.
 *
 * This is the one operation both loads and column-set changes need --
 * the future add/remove-columns setting calls exactly this.
 */
export function reconcileWidths(
  specs: ColumnSpec[],
  stored: ColumnWidths,
  containerPx: number
): ColumnWidths {
  const base = initialWidths(specs, containerPx)
  const merged: ColumnWidths = {}
  for (const d of specs) {
    const kept = stored[d.id]
    merged[d.id] =
      d.flex === 0
        ? d.min
        : Number.isFinite(kept)
          ? Math.max(d.min, kept)
          : base[d.id]
  }
  return fitWidths(specs, merged, containerPx)
}

/**
 * Apply a bar drag to a starting layout. The bar is named by the flex
 * column it follows; a positive delta moves it right. The column nearest
 * the bar on the growing side takes all the growth; the shrinking side
 * gives space nearest-first, cascading past columns at their min. The
 * delta is clamped to what the shrinking side can supply, so total width
 * is conserved and a fully-compressed side stops the handle.
 *
 * Pure over the starting layout: callers pass the widths captured at
 * drag start and the cumulative pointer delta, so repeated calls cannot
 * drift.
 */
export function resizeBar(
  specs: ColumnSpec[],
  start: ColumnWidths,
  barId: string,
  deltaPx: number
): ColumnWidths {
  const i = specs.findIndex(d => d.id === barId)
  if (i < 0 || specs[i].flex === 0 || deltaPx === 0) return start

  const before = specs
    .slice(0, i + 1)
    .filter(d => d.flex > 0)
    .reverse()
  const beyond = specs.slice(i + 1).filter(d => d.flex > 0)
  if (beyond.length === 0) return start

  const shrinkSide = deltaPx > 0 ? beyond : before
  const growId = deltaPx > 0 ? before[0].id : beyond[0].id

  const available = shrinkSide.reduce(
    (s, d) => s + Math.max(0, (start[d.id] ?? d.min) - d.min),
    0
  )
  const take = Math.min(Math.abs(deltaPx), available)
  if (take <= 0) return start

  const next: ColumnWidths = { ...start }
  next[growId] += take
  let remaining = take
  for (const d of shrinkSide) {
    const give = Math.min(remaining, next[d.id] - d.min)
    next[d.id] -= give
    remaining -= give
    if (remaining <= 0) break
  }
  return next
}

export type ColumnBarPosition = {
  id: string
  /** Current boundary position in px from the row's left edge. */
  left: number
  /** Furthest reachable left position without crossing a column minimum. */
  min: number
  /** Furthest reachable right position without crossing a column minimum. */
  max: number
}

/**
 * Where the resize bars sit and the range each can reach: after each flex
 * column that has another flex column somewhere beyond it. The range uses
 * the same per-side slack as resizeBar, so ARIA values and pointer geometry
 * cannot disagree when a resize cascades across columns.
 */
export function barPositions(
  specs: ColumnSpec[],
  widths: ColumnWidths
): ColumnBarPosition[] {
  const out: ColumnBarPosition[] = []
  let cumulative = 0
  for (let i = 0; i < specs.length; i++) {
    cumulative += widths[specs[i].id] ?? 0
    if (specs[i].flex > 0 && specs.slice(i + 1).some(d => d.flex > 0)) {
      const beforeSlack = specs
        .slice(0, i + 1)
        .reduce(
          (sum, d) =>
            sum +
            (d.flex > 0 ? Math.max(0, (widths[d.id] ?? d.min) - d.min) : 0),
          0
        )
      const beyondSlack = specs
        .slice(i + 1)
        .reduce(
          (sum, d) =>
            sum +
            (d.flex > 0 ? Math.max(0, (widths[d.id] ?? d.min) - d.min) : 0),
          0
        )
      out.push({
        id: specs[i].id,
        left: cumulative,
        min: cumulative - beforeSlack,
        max: cumulative + beyondSlack,
      })
    }
  }
  return out
}

/**
 * Attach pointer-based resize to a bar. Mechanics (capture, cursor,
 * selection) come from startDrag; the geometry is resizeBar over the
 * layout captured at drag start.
 */
export function startColumnResize(
  specs: ColumnSpec[],
  startWidths: ColumnWidths,
  barId: string,
  e: PointerEvent,
  onResize: (widths: ColumnWidths) => void,
  onEnd: () => void
): DragHandle {
  return startDrag(e, {
    axis: 'x',
    onMove: delta => onResize(resizeBar(specs, startWidths, barId, delta)),
    onEnd,
  })
}
