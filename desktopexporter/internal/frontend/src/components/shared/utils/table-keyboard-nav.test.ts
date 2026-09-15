import { describe, expect, it } from 'vitest'
import { keyDeltaFor, resolveNextPos } from './table-keyboard-nav'

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

  it('clamps relative and absolute navigation to the available positions', () => {
    expect(resolveNextPos({ kind: 'relative', offset: 8 }, 3, 5)).toBe(5)
    expect(resolveNextPos({ kind: 'relative', offset: -8 }, 3, 5)).toBe(0)
    expect(resolveNextPos({ kind: 'absolute', position: 'last' }, 3, 5)).toBe(5)
  })
})
