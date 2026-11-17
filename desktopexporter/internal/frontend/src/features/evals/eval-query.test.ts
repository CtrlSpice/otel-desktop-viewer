import { expect, it } from 'vitest'
import { evalQuery } from './eval-query'

it('preserves opaque IDs and filters when opening an old selection', () => {
  const id = '["dataset","run","native-id"]'
  expect(
    evalQuery({ run: id, rep: 'native-result', row: `run/${id}`, q: 'Fail' })
  ).toEqual({
    experiment: id,
    result: 'native-result',
    row: `experiment/${id}`,
    q: 'Fail',
  })
  expect(evalQuery({ row: `rep/${id}` }).row).toBe(`result/${id}`)
  expect(evalQuery({ row: `case/${id}` }).row).toBe(`case/${id}`)
})

it('prefers current selection keys over stale aliases', () => {
  expect(
    evalQuery({
      run: 'old',
      experiment: 'new',
      rep: 'old-result',
      result: 'new-result',
    })
  ).toEqual({
    experiment: 'new',
    result: 'new-result',
  })
})
