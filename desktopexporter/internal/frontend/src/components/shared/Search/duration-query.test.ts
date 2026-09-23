import { describe, expect, it } from 'vitest'
import { normalizeDuration, normalizeDurationList } from './duration-query'

describe('normalizeDuration', () => {
  it.each([
    ['1.5h', '5400000000000'],
    ['0.5ns', '1'],
    ['0.499999999999999999ns', '0'],
    ['0s', '0'],
  ])('normalizes %s exactly', (input, expected) => {
    expect(normalizeDuration(input)).toEqual({ ok: true, value: expected })
  })

  it.each(['-1ms', '9223372036854775808ns'])('rejects %s', input => {
    expect(normalizeDuration(input)).toEqual({
      ok: false,
      error: expect.stringMatching(/^Invalid duration:/),
    })
  })
})

describe('normalizeDurationList', () => {
  it('normalizes typed values without changing their order', () => {
    expect(normalizeDurationList(['1s', '0.5ns', '0s'])).toEqual({
      ok: true,
      value: ['1000000000', '1', '0'],
    })
  })

  it.each([
    [[], { ok: false, error: 'IN and NOT IN require a nonempty list' }],
    [
      ['1s', 'bad'],
      {
        ok: false,
        error: 'Invalid duration at list element 2',
        index: 1,
      },
    ],
    [
      ['9223372036854775808ns'],
      {
        ok: false,
        error: 'Invalid duration at list element 1',
        index: 0,
      },
    ],
  ])('rejects invalid typed list %#', (input, expected) => {
    expect(normalizeDurationList(input)).toEqual(expected)
  })
})
