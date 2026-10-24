// @vitest-environment jsdom
import { afterEach, expect, it, vi } from 'vitest'
import {
  loadAttributeColumns,
  loadPinnedAttributeColumns,
  saveAttributeColumns,
  visibleAttributeColumns,
} from './attribute-columns'

afterEach(() => vi.unstubAllGlobals())

it('defaults to a pinned Name, distinct values and records without adding breakdown columns', () => {
  expect(loadAttributeColumns()).toEqual([
    'name',
    'values',
    'count',
    'frequency',
    'copy',
  ])
  expect(loadPinnedAttributeColumns()).toEqual(['name'])
})

it('remembers visibility and pins independently, retains Name, and ignores invalid IDs', () => {
  saveAttributeColumns(
    ['values', 'count', 'copy', 'not-a-column'],
    ['count', 'copy', 'not-a-column']
  )
  expect(loadAttributeColumns()).toEqual(['name', 'values', 'count', 'copy'])
  expect(loadPinnedAttributeColumns()).toEqual(['count'])
  const visible = visibleAttributeColumns(
    loadAttributeColumns(),
    loadPinnedAttributeColumns()
  )
  expect(visible.map(column => column.id)).toEqual([
    'count',
    'name',
    'values',
    'copy',
  ])
})

it('keeps hidden pin choices without forcing them visible, and Copy stays last', () => {
  const selected = ['name', 'count', 'copy']
  const pinned = ['name', 'type', 'count', 'copy']
  expect(
    visibleAttributeColumns(selected, pinned).map(column => column.id)
  ).toEqual(['name', 'count', 'copy'])
  expect(selected).toEqual(['name', 'count', 'copy'])
  expect(pinned).toEqual(['name', 'type', 'count', 'copy'])
})

it('can unpin every column and validates malformed saved preferences', () => {
  saveAttributeColumns(['name'], [])
  expect(loadPinnedAttributeColumns()).toEqual([])
  localStorage.setItem('home-attribute-columns', '[null]')
  localStorage.setItem('home-attribute-pinned-columns', 'not JSON')
  expect(loadAttributeColumns()).toEqual([
    'name',
    'values',
    'count',
    'frequency',
    'copy',
  ])
  expect(loadPinnedAttributeColumns()).toEqual(['name'])
})

it('keeps working when browser storage is blocked', () => {
  vi.stubGlobal('localStorage', {
    getItem: () => {
      throw new Error('blocked')
    },
    setItem: () => {
      throw new Error('blocked')
    },
  })
  expect(loadAttributeColumns()).toEqual([
    'name',
    'values',
    'count',
    'frequency',
    'copy',
  ])
  expect(loadPinnedAttributeColumns()).toEqual(['name'])
  expect(() => saveAttributeColumns(['name', 'count'], ['name'])).not.toThrow()
})
