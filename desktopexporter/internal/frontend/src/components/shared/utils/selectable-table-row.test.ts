// @vitest-environment jsdom
import { afterEach, expect, it, vi } from 'vitest'
import { selectableTableRow } from './selectable-table-row'

afterEach(() => {
  document.body.replaceChildren()
  document.getSelection()?.removeAllRanges()
})

function grid() {
  const table = document.createElement('table')
  table.setAttribute('role', 'grid')
  const body = table.createTBody()
  const rows = Array.from({ length: 3 }, () => {
    const row = body.insertRow()
    row.dataset.selectableRow = ''
    row.insertCell().textContent = 'received value'
    return row
  })
  document.body.append(table)
  return rows
}

it('selects and focuses a row from cell background, not from controls or selected text', () => {
  const [row] = grid()
  const onSelect = vi.fn()
  const action = selectableTableRow(row, { selected: false, onSelect })
  const cell = row.cells[0]
  cell.dispatchEvent(new MouseEvent('click', { bubbles: true, detail: 1 }))
  expect(onSelect).toHaveBeenCalledOnce()
  expect(row).toHaveFocus()
  const copy = document.createElement('button')
  cell.append(copy)
  copy.dispatchEvent(new MouseEvent('click', { bubbles: true, detail: 1 }))
  expect(onSelect).toHaveBeenCalledOnce()
  const range = document.createRange()
  range.selectNodeContents(cell)
  document.getSelection()?.removeAllRanges()
  document.getSelection()?.addRange(range)
  expect(document.getSelection()?.toString()).toBe('received value')
  cell.dispatchEvent(new MouseEvent('click', { bubbles: true, detail: 1 }))
  expect(onSelect).toHaveBeenCalledOnce()
  action?.update?.({ selected: true, onSelect })
  expect(row).toHaveAttribute('aria-selected', 'true')
  expect(row).toHaveAttribute('tabindex', '0')
  action?.destroy?.()
  document.getSelection()?.removeAllRanges()
  cell.click()
  expect(onSelect).toHaveBeenCalledOnce()
})

it('supports row keyboard selection and navigation without intercepting native cell controls', () => {
  const [first, hidden, last] = grid()
  hidden.hidden = true
  const onSelect = vi.fn()
  const firstAction = selectableTableRow(first, { selected: true, onSelect })
  const lastAction = selectableTableRow(last, { selected: false, onSelect })
  first.focus()
  first.dispatchEvent(
    new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true })
  )
  expect(last).toHaveFocus()
  last.dispatchEvent(
    new KeyboardEvent('keydown', { key: 'Enter', bubbles: true })
  )
  expect(onSelect).toHaveBeenCalledOnce()
  last.dispatchEvent(
    new KeyboardEvent('keydown', { key: 'Home', bubbles: true })
  )
  expect(first).toHaveFocus()
  const button = document.createElement('button')
  first.cells[0].append(button)
  button.dispatchEvent(
    new KeyboardEvent('keydown', { key: ' ', bubbles: true })
  )
  expect(onSelect).toHaveBeenCalledOnce()
  firstAction?.destroy?.()
  lastAction?.destroy?.()
})
