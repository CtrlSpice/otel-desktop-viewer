// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import {
  clampNavTargetIndex,
  findItemIndexByID,
  resolveFallbackIndex,
} from '@/contexts/signal-list-page.svelte'

describe('findItemIndexByID', () => {
  const items = [{ id: 'a' }, { id: 'b' }, { id: 'c' }]
  const getID = (item: { id: string }) => item.id

  it('returns -1 when id is null', () => {
    expect(findItemIndexByID(items, null, getID)).toBe(-1)
  })

  it('returns -1 when id is not in the list', () => {
    expect(findItemIndexByID(items, 'missing', getID)).toBe(-1)
  })

  it('returns the matching index', () => {
    expect(findItemIndexByID(items, 'b', getID)).toBe(1)
  })
})

describe('clampNavTargetIndex', () => {
  it('returns -1 when the list is empty', () => {
    expect(clampNavTargetIndex(0, 1, 0)).toBe(-1)
  })

  it('returns -1 when nothing is selected', () => {
    expect(clampNavTargetIndex(-1, 1, 3)).toBe(-1)
  })

  it('clamps at the first row', () => {
    expect(clampNavTargetIndex(0, -1, 3)).toBe(0)
  })

  it('clamps at the last row', () => {
    expect(clampNavTargetIndex(2, 1, 3)).toBe(2)
  })

  it('moves within range', () => {
    expect(clampNavTargetIndex(1, 1, 3)).toBe(2)
  })
})

describe('resolveFallbackIndex', () => {
  it('returns 0 for an empty list', () => {
    expect(resolveFallbackIndex(5, 0)).toBe(0)
  })

  it('clamps lastValidIndex to list length - 1', () => {
    expect(resolveFallbackIndex(9, 3)).toBe(2)
  })

  it('preserves lastValidIndex when in range', () => {
    expect(resolveFallbackIndex(1, 5)).toBe(1)
  })
})
