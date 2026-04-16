type KeyDelta =
  | { kind: 'relative'; offset: number }
  | { kind: 'absolute'; position: 'first' | 'last' }

const PAGE_STEP = 10

const KEY_DELTAS: Record<string, KeyDelta> = {
  ArrowDown: { kind: 'relative', offset: 1 },
  j: { kind: 'relative', offset: 1 },
  ArrowUp: { kind: 'relative', offset: -1 },
  k: { kind: 'relative', offset: -1 },
  PageDown: { kind: 'relative', offset: PAGE_STEP },
  PageUp: { kind: 'relative', offset: -PAGE_STEP },
  Home: { kind: 'absolute', position: 'first' },
  End: { kind: 'absolute', position: 'last' },
}

function resolveNextPos(
  delta: KeyDelta,
  currentPos: number,
  lastPos: number
): number {
  const raw =
    delta.kind === 'absolute'
      ? delta.position === 'first'
        ? 0
        : lastPos
      : currentPos + delta.offset
  return Math.max(0, Math.min(raw, lastPos))
}

function shouldHandle(el: HTMLElement | null, root: HTMLElement): boolean {
  if (!el || !root.contains(el)) return false
  if (el.closest('input, textarea, select, [contenteditable="true"]'))
    return false
  if (el.closest('button')) return false
  return true
}

function escapeForSelector(value: string): string {
  return typeof CSS !== 'undefined' && typeof CSS.escape === 'function'
    ? CSS.escape(value)
    : value.replace(/\\/g, '\\\\').replace(/"/g, '\\"')
}

export interface TableNavOptions {
  /** The data-* attribute name on each <tr> that holds the row ID (without the `data-` prefix). */
  rowIdAttr: string
  /** Called when keyboard navigation lands on a row. */
  onSelect: (id: string) => void
  /**
   * Optional: called for Enter/Space on a focused row.
   * If not provided, `onSelect` is called instead.
   */
  onActivate?: (id: string) => void
  /** Override the page step for PageUp/PageDown (default 10). */
  pageStep?: number
  /**
   * Extra key handler that runs before the default nav.
   * Receives the current row's ID (null if nothing focused).
   * Return `true` to suppress default nav for this event.
   */
  onKey?: (e: KeyboardEvent, currentId: string | null) => boolean
  /** Skip rows with aria-hidden="true" (useful for collapsible trees). Default false. */
  skipHidden?: boolean
}

/**
 * Svelte action that adds keyboard navigation to a table.
 *
 * Attach to the `<table>` element. Each navigable `<tr>` inside must have:
 *   - a `data-{rowIdAttr}` attribute with the row's unique ID
 *   - `tabindex="0"` (or `-1` — the action will focus rows programmatically)
 *
 * Supported keys: ArrowUp/Down, j/k, PageUp/Down, Home/End, Enter/Space.
 */
export function tableNav(node: HTMLElement, opts: TableNavOptions) {
  let current = opts

  const selector = () => `tr[data-${current.rowIdAttr}]`
  const dataKey = () =>
    current.rowIdAttr.replace(/-([a-z])/g, (_, c: string) => c.toUpperCase())

  function getRows(): HTMLElement[] {
    const all = node.querySelectorAll<HTMLElement>(selector())
    if (!current.skipHidden) return Array.from(all)
    return Array.from(all).filter(r => r.getAttribute('aria-hidden') !== 'true')
  }

  function focusAndScroll(row: HTMLElement) {
    row.focus()
    row.scrollIntoView({ block: 'nearest' })
  }

  function handleKeydown(e: KeyboardEvent) {
    if (!shouldHandle(e.target as HTMLElement | null, node)) return

    const rows = getRows()
    if (rows.length === 0) return

    const focused = document.activeElement as HTMLElement | null
    const currentIdx = focused ? rows.indexOf(focused) : -1
    const currentId =
      currentIdx >= 0 ? (focused!.dataset[dataKey()] ?? null) : null

    if (current.onKey?.(e, currentId)) return

    if (e.key === 'Enter' || e.key === ' ') {
      if (currentId) {
        e.preventDefault()
        ;(current.onActivate ?? current.onSelect)(currentId)
      }
      return
    }

    const step = current.pageStep ?? PAGE_STEP
    const deltas: Record<string, KeyDelta> =
      step === PAGE_STEP
        ? KEY_DELTAS
        : {
            ...KEY_DELTAS,
            PageDown: { kind: 'relative', offset: step },
            PageUp: { kind: 'relative', offset: -step },
          }

    const delta = deltas[e.key]
    if (!delta) return

    e.preventDefault()

    if (currentIdx < 0) {
      const first = rows[0]
      const id = first.dataset[dataKey()]
      if (id) {
        current.onSelect(id)
        focusAndScroll(first)
      }
      return
    }

    const nextIdx = resolveNextPos(delta, currentIdx, rows.length - 1)
    if (nextIdx === currentIdx) return

    const nextRow = rows[nextIdx]
    const nextId = nextRow.dataset[dataKey()]
    if (nextId) {
      current.onSelect(nextId)
      focusAndScroll(nextRow)
    }
  }

  node.addEventListener('keydown', handleKeydown)

  return {
    update(newOpts: TableNavOptions) {
      current = newOpts
    },
    destroy() {
      node.removeEventListener('keydown', handleKeydown)
    },
  }
}

export { escapeForSelector, resolveNextPos, type KeyDelta }
