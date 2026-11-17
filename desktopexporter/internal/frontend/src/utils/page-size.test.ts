import { afterEach, describe, expect, it, vi } from 'vitest'
import { isPageSize, loadPageSize, savePageSize } from './page-size'

afterEach(() => vi.unstubAllGlobals())

describe('page-size preferences', () => {
  it.each([25, 50, 100])('accepts the supported size %s', size => {
    expect(isPageSize(size)).toBe(true)
  })

  it.each([0, -25, 250, 1.5, NaN, Infinity])(
    'rejects %s without creating an unbounded page',
    size => {
      expect(isPageSize(size)).toBe(false)
    }
  )

  it('retains supported choices and defaults malformed storage to 25', () => {
    const getItem = vi.fn().mockReturnValue('50')
    const setItem = vi.fn()
    vi.stubGlobal('localStorage', { getItem, setItem })
    expect(loadPageSize('test')).toBe(50)
    getItem.mockReturnValue('1000000')
    expect(loadPageSize('test')).toBe(25)
    savePageSize('test', 100)
    expect(setItem).toHaveBeenCalledExactlyOnceWith('test', '100')
  })

  it('works for the current view when storage is blocked', () => {
    vi.stubGlobal('localStorage', {
      getItem: () => {
        throw new Error('blocked')
      },
      setItem: () => {
        throw new Error('blocked')
      },
    })
    expect(loadPageSize('test')).toBe(25)
    expect(() => savePageSize('test', 50)).not.toThrow()
  })
})
