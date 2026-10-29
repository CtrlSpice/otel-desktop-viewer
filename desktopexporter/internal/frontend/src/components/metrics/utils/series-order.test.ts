import { expect, it } from 'vitest'
import { validateSeriesOrder } from './series-order'

it('rejects incomplete, duplicate and foreign ordered cohorts', () => {
  for (const order of [
    [],
    ['a'],
    ['a', 'a'],
    ['a', 'foreign'],
    ['a', 'b', 'foreign'],
  ]) {
    expect(() => validateSeriesOrder(order, ['a', 'b'])).toThrow(
      'Series changed'
    )
  }
  expect(() => validateSeriesOrder(['b', 'a'], ['a', 'b'])).not.toThrow()
})
