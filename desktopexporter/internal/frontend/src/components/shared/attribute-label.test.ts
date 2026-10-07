import { describe, expect, it } from 'vitest'
import { attributeValueCanonical, attributeValueLabel } from './attribute-label'

describe('attribute value labels', () => {
  it.each([
    [-0, '-0'],
    [0, '0'],
    [NaN, 'NaN'],
    [Infinity, 'Infinity'],
    [-Infinity, '-Infinity'],
  ])('keeps the double %s readable as %s', (value, label) => {
    expect(attributeValueLabel({ kind: 'double', value })).toBe(label)
  })

  it('keeps signed zero distinct in nested labels and series comparisons', () => {
    expect(
      attributeValueLabel({
        kind: 'map',
        value: [
          {
            key: 'zeros',
            value: {
              kind: 'array',
              value: [
                { kind: 'double', value: -0 },
                { kind: 'double', value: 0 },
              ],
            },
          },
        ],
      })
    ).toBe('{zeros: [-0, 0]}')
    expect(attributeValueCanonical({ kind: 'double', value: -0 })).not.toBe(
      attributeValueCanonical({ kind: 'double', value: 0 })
    )
  })
})
