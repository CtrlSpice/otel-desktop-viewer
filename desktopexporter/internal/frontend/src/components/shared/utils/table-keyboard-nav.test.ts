// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { tableNav } from './table-keyboard-nav'

function setupTable() {
  const table = document.createElement('table')
  const first = table.insertRow()
  const second = table.insertRow()
  first.dataset.rowId = 'first'
  second.dataset.rowId = 'second'
  first.tabIndex = 0
  second.tabIndex = -1
  first.scrollIntoView = vi.fn()
  second.scrollIntoView = vi.fn()
  document.body.appendChild(table)
  return { table, first, second }
}

afterEach(() => {
  document.body.innerHTML = ''
})

describe('tableNav DOM targets', () => {
  it('derives the current row from the focused rows before navigating', () => {
    const { table, first, second } = setupTable()
    const onSelect = vi.fn()
    const action = tableNav(table, { rowIdAttr: 'row-id', onSelect })
    first.focus()

    first.dispatchEvent(
      new KeyboardEvent('keydown', {
        key: 'ArrowDown',
        bubbles: true,
        cancelable: true,
      })
    )

    expect(onSelect).toHaveBeenCalledWith('second')
    expect(second).toHaveFocus()
    action.destroy()
  })

  it('accepts a delegated SVG target while deriving focus from owned rows', () => {
    const { table, first, second } = setupTable()
    const icon = document.createElementNS('http://www.w3.org/2000/svg', 'svg')
    first.appendChild(icon)
    const onSelect = vi.fn()
    const action = tableNav(table, { rowIdAttr: 'row-id', onSelect })
    first.focus()

    icon.dispatchEvent(
      new KeyboardEvent('keydown', {
        key: 'ArrowDown',
        bubbles: true,
        cancelable: true,
      })
    )

    expect(onSelect).toHaveBeenCalledWith('second')
    expect(second).toHaveFocus()
    action.destroy()
  })
})
