import { describe, expect, it } from 'vitest'
import { keyDeltaFor } from './table-keyboard-nav'

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
