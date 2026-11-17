import { describe, expect, it } from 'vitest'
import { updateColumnSelection } from './column-selection'

describe('column group selection', () => {
  const options = [
    { id: 'case', disabled: true },
    { id: 'a' },
    { id: 'b' },
    { id: 'other' },
  ]
  it('removes a whole group while retaining required and unrelated columns', () => {
    expect(
      updateColumnSelection(
        ['case', 'a', 'b', 'other'],
        options,
        ['case', 'a', 'b'],
        false
      )
    ).toEqual(['case', 'other'])
  })
  it('selects the whole group without duplicating current choices', () => {
    expect(
      updateColumnSelection(['case', 'a', 'other'], options, ['a', 'b'], true)
    ).toEqual(['case', 'a', 'other', 'b'])
  })
  it('retains the first current column when a table requires one', () => {
    expect(
      updateColumnSelection(['a', 'b'], options, ['a', 'b'], false, 1)
    ).toEqual(['a'])
  })
})
