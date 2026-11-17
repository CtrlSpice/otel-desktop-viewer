// @vitest-environment jsdom
import { afterEach, expect, it } from 'vitest'
import { loadLogColumns, logColumnRequests } from './log-table'

afterEach(() => localStorage.clear())

it('salvages valid saved columns without changing literal attribute keys or order', () => {
  const literal = JSON.stringify(['attribute', 'resource', ' name.[] '])
  const empty = JSON.stringify(['attribute', 'log', ''])
  localStorage.setItem(
    'logs-table-columns',
    JSON.stringify([
      42,
      null,
      'severity',
      literal,
      empty,
      literal,
      JSON.stringify(['field', 'log', 'not-a-field']),
      JSON.stringify(['attribute', 'other', 'key']),
      JSON.stringify(['attribute', 'log', 42]),
      JSON.stringify(['attribute', 'log', 'key', 'extra']),
    ])
  )
  const columns = loadLogColumns()
  expect(columns).toEqual(['severity', literal, empty])
  expect(logColumnRequests(columns)).toEqual([
    { kind: 'attribute', source: 'resource', key: ' name.[] ' },
    { kind: 'attribute', source: 'log', key: '' },
  ])
})
