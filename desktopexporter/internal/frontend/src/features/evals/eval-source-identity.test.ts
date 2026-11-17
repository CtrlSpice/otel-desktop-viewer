import { expect, it } from 'vitest'
import { evalSourceIdentities } from './eval-source-identity'

it.each([
  ['0.10000000000000001', '0.1', false],
  ['1e-400', '0', false],
  ['-0', '0', false],
  ['1.00', '10e-1', true],
  ['1e-400', '10e-401', true],
  ['-0.00', '-0e50', true],
  ['0e99', '0', true],
  ['"0.1"', '0.1', false],
  ['{"x":1,"y":"\\u0061"}', '{"y":"a","x":1.0}', true],
  ['{"x":9,"x":1}', '{"x":1}', true],
])('compares exact source identities: %s and %s', (a, b, equal) => {
  const identity = evalSourceIdentities(
    `{"results":{"results":[{"testCase":${a}},{"testCase":${b}}]}}`
  )
  expect(identity(0, '/testCase') === identity(1, '/testCase')).toBe(equal)
})
