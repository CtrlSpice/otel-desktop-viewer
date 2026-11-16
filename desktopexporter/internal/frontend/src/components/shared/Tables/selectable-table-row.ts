import type { Action } from 'svelte/action'

type Selection = { selected: boolean; onSelect: () => void }

/** Row selection for a native table exposed as an ARIA grid. Native cell controls remain independent. */
export const selectableTableRow: Action<HTMLTableRowElement, Selection> = (
  row,
  selection
) => {
  const originalTabIndex = row.getAttribute('tabindex')
  const originalSelected = row.getAttribute('aria-selected')
  function sync() {
    row.tabIndex = selection.selected ? 0 : -1
    row.setAttribute('aria-selected', String(selection.selected))
  }
  function click(event: MouseEvent) {
    if (
      event.defaultPrevented ||
      (event.detail !== 0 &&
        row.ownerDocument.getSelection()?.isCollapsed === false)
    )
      return
    if (
      event.target instanceof Element &&
      event.target.closest(
        'button, a, input, select, textarea, [role="button"]'
      )
    )
      return
    selection.onSelect()
    row.focus({ preventScroll: true })
  }
  function keydown(event: KeyboardEvent) {
    if (event.target !== row) return
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault()
      selection.onSelect()
      return
    }
    if (!['ArrowUp', 'ArrowDown', 'Home', 'End'].includes(event.key)) return
    const rows = [
      ...(row
        .closest('table[role="grid"]')
        ?.querySelectorAll<HTMLTableRowElement>('tr[data-selectable-row]') ??
        []),
    ].filter(item => !item.hidden)
    const index = rows.indexOf(row)
    const next =
      event.key === 'Home'
        ? 0
        : event.key === 'End'
          ? rows.length - 1
          : index + (event.key === 'ArrowUp' ? -1 : 1)
    event.preventDefault()
    rows[Math.max(0, Math.min(next, rows.length - 1))]?.focus()
  }
  sync()
  row.addEventListener('click', click)
  row.addEventListener('keydown', keydown)
  return {
    update(next) {
      selection = next
      sync()
    },
    destroy() {
      row.removeEventListener('click', click)
      row.removeEventListener('keydown', keydown)
      if (originalTabIndex === null) row.removeAttribute('tabindex')
      else row.setAttribute('tabindex', originalTabIndex)
      if (originalSelected === null) row.removeAttribute('aria-selected')
      else row.setAttribute('aria-selected', originalSelected)
    },
  }
}
