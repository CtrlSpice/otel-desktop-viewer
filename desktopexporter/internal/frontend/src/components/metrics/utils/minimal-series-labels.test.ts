import { expect, it } from 'vitest'
import type { LegendTimeseries } from '@/types/metric-chart-types'
import type { Attribute } from '@/types/api-types'
import {
  minimalSeriesLabels,
  minimalSeriesTable,
  compareSeriesCells,
} from './minimal-series-labels'

function row(key: string, values: Record<string, string>): LegendTimeseries {
  return {
    key,
    attributes: Object.entries(values).map(([key, value]) => ({
      key,
      value: { kind: 'string', value },
    })),
  }
}

it('sorts numeric attributes without losing int64 precision and keeps typed filter identities', () => {
  const rows: LegendTimeseries[] = [
    {
      key: 'big',
      attributes: [
        { key: 'n', value: { kind: 'int64', value: 9007199254740993n } },
      ],
    },
    {
      key: 'small',
      attributes: [
        { key: 'n', value: { kind: 'int64', value: 9007199254740992n } },
      ],
    },
    {
      key: 'negative',
      attributes: [{ key: 'n', value: { kind: 'double', value: -10 } }],
    },
    {
      key: 'string',
      attributes: [{ key: 'n', value: { kind: 'string', value: '-10' } }],
    },
  ]
  const column = minimalSeriesTable(rows).columns[0]!
  expect(
    compareSeriesCells(column.cells.get('big')!, column.cells.get('small')!)
  ).toBeGreaterThan(0)
  expect(
    compareSeriesCells(
      column.cells.get('negative')!,
      column.cells.get('small')!
    )
  ).toBeLessThan(0)
  expect(column.cells.get('negative')!.signature).not.toBe(
    column.cells.get('string')!.signature
  )
})

it('uses model and type instead of redundant session IDs, stably across row order', () => {
  const rows = ['haiku', 'sonnet', 'opus'].flatMap(model =>
    ['input', 'output', 'cacheRead', 'cacheCreation'].map(type =>
      row(`${model}/${type}`, {
        model,
        type,
        'session.id': `long-session-identifier-for-${model}`,
        common: 'same',
      })
    )
  )
  const labels = minimalSeriesLabels(rows)
  expect(labels.get('haiku/output')).toEqual({
    text: 'model: haiku · type: output',
    needsIdentity: false,
  })
  expect(new Set([...labels.values()].map(label => label.text)).size).toBe(12)
  expect(minimalSeriesLabels([...rows].reverse())).toEqual(labels)
})

it('keeps missing, empty and differently typed values visibly distinct', () => {
  const attributes: Attribute[][] = [
    [],
    [{ key: 'value', value: { kind: 'empty', value: null } }],
    [{ key: 'value', value: { kind: 'string', value: '1' } }],
    [{ key: 'value', value: { kind: 'double', value: 1 } }],
    [{ key: 'value', value: { kind: 'string', value: '[absent]' } }],
  ]
  const labels = minimalSeriesLabels(
    attributes.map((attributes, index) => ({ key: String(index), attributes }))
  )
  expect(new Set([...labels.values()].map(label => label.text)).size).toBe(5)
  expect([...labels.values()].every(label => !label.needsIdentity)).toBe(true)
})

it('preserves duplicate keys and distinguishes nested values with ambiguous plain text', () => {
  const nested = (values: string[]): Attribute => ({
    key: 'nested',
    value: {
      kind: 'array',
      value: values.map(value => ({ kind: 'string', value })),
    },
  })
  const rows: LegendTimeseries[] = [
    { key: 'a', attributes: [nested(['a, b', 'c'])] },
    { key: 'b', attributes: [nested(['a', 'b, c'])] },
  ]
  const labels = minimalSeriesLabels(rows)
  expect(labels.get('a')!.text).not.toBe(labels.get('b')!.text)
  const duplicates = [row('a', { x: 'one' }), row('b', { x: 'one' })]
  duplicates[1]!.attributes.push({
    key: 'x',
    value: { kind: 'string', value: 'two' },
  })
  expect(minimalSeriesLabels(duplicates).get('a')!.text).toBe('x: one')
  expect(minimalSeriesLabels(duplicates).get('b')!.text).toBe('x: one, two')
})

it('requests an identity suffix only when attributes cannot distinguish rows', () => {
  const rows = [row('a', { same: 'value' }), row('b', { same: 'value' })]
  expect([...minimalSeriesLabels(rows).values()]).toEqual([
    { text: 'default series', needsIdentity: true },
    { text: 'default series', needsIdentity: true },
  ])
  expect(minimalSeriesLabels([rows[0]!]).get('a')).toEqual({
    text: 'default series',
    needsIdentity: false,
  })
  expect(minimalSeriesLabels([]).size).toBe(0)
})

it('finds the exact minimum on varied datasets, checked against exhaustive subsets', () => {
  let seed = 711
  const next = () => (seed = (Math.imul(seed, 1664525) + 1013904223) >>> 0)
  for (let sample = 0; sample < 64; sample++) {
    const values = Array.from({ length: 8 }, () =>
      Array.from({ length: 5 }, () => String(next() % 3))
    )
    const distinct = new Set(values.map(value => JSON.stringify(value))).size
    let minimum = 5
    for (let mask = 0; mask < 32; mask++) {
      const keys = [0, 1, 2, 3, 4].filter(key => mask & (1 << key))
      if (
        new Set(values.map(row => JSON.stringify(keys.map(key => row[key]))))
          .size === distinct
      )
        minimum = Math.min(minimum, keys.length)
    }
    const rows = values.map((values, index) =>
      row(
        String(index),
        Object.fromEntries(values.map((value, key) => [`k${key}`, value]))
      )
    )
    const labels = [...minimalSeriesLabels(rows).values()]
    expect(
      labels[0]!.text === 'default series'
        ? 0
        : labels[0]!.text.split(' · ').length
    ).toBe(minimum)
    expect(new Set(labels.map(label => label.text)).size).toBe(distinct)
  }
})
