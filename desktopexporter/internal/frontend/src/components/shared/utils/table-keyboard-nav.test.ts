// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { keyDeltaFor, tableNav } from './table-keyboard-nav'

function setupTable(ownerDocument: Document = document) {
  const table = ownerDocument.createElement('table')
  const first = table.insertRow()
  const second = table.insertRow()
  first.dataset.rowId = 'first'
  second.dataset.rowId = 'second'
  first.tabIndex = 0
  second.tabIndex = -1
  first.scrollIntoView = vi.fn()
  second.scrollIntoView = vi.fn()
  ownerDocument.body.appendChild(table)
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

  it('navigates rows owned by a foreign document', () => {
    const iframe = document.createElement('iframe')
    document.body.appendChild(iframe)
    const foreignDocument = iframe.contentDocument
    if (!foreignDocument) throw new Error('Expected iframe document')
    const { table, first, second } = setupTable(foreignDocument)
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

    expect(table).not.toBeInstanceOf(HTMLElement)
    expect(onSelect).toHaveBeenCalledWith('second')
    expect(foreignDocument.activeElement).toBe(second)
    action.destroy()
  })
})

describe('keyDeltaFor', () => {
  it('resolves directional and absolute navigation keys', () => {
    expect(keyDeltaFor('ArrowDown')).toEqual({ kind: 'relative', offset: 1 })
    expect(keyDeltaFor('Home')).toEqual({ kind: 'absolute', position: 'first' })
  })

  it('uses the caller page step', () => {
    expect(keyDeltaFor('PageDown', 8)).toEqual({ kind: 'relative', offset: 8 })
    expect(keyDeltaFor('PageUp', 8)).toEqual({ kind: 'relative', offset: -8 })
  })

  it('does not treat unknown or prototype keys as navigation', () => {
    expect(keyDeltaFor('nope')).toBeUndefined()
    expect(keyDeltaFor('toString')).toBeUndefined()
    expect(keyDeltaFor('constructor')).toBeUndefined()
  })
})
