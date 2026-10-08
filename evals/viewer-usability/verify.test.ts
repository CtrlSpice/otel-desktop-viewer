import test from 'node:test'
import assert from 'node:assert/strict'
import { normalize } from './verify.ts'

test('SQL answer comparison ignores row order and nested object key order', () => {
  const received = [
    {
      a: 1,
      b: 2,
      nested: { exact: '18446744073709551615', time: '1791360000123456789' },
    },
    { a: 2, b: 1 },
  ]
  const expected = [
    { b: 1, a: 2 },
    {
      nested: { time: '1791360000123456789', exact: '18446744073709551615' },
      b: 2,
      a: 1,
    },
  ]
  assert.deepEqual(normalize(received), normalize(expected))
  assert.deepEqual(
    normalize([
      { a: 1, b: 2 },
      { a: 2, b: 1 },
    ]),
    normalize([
      { b: 2, a: 1 },
      { b: 1, a: 2 },
    ])
  )
  assert.notDeepEqual(normalize([1, 1, 2]), normalize([1, 2, 2]))
  assert.deepEqual(received[0].nested, {
    exact: '18446744073709551615',
    time: '1791360000123456789',
  })
})
