// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { rejectionLabel } from './HomePage.svelte'

describe('rejectionLabel', () => {
  it('labels known rejection kinds', () => {
    expect(rejectionLabel('span_already_stored')).toBe('duplicate span id')
    expect(rejectionLabel('span_refused')).toBe('rejected by the store')
  })

  it('falls back to unknown and prototype-named kinds', () => {
    expect(rejectionLabel('new_rejection')).toBe('new_rejection')
    expect(rejectionLabel('toString')).toBe('toString')
    expect(rejectionLabel('constructor')).toBe('constructor')
  })
})
