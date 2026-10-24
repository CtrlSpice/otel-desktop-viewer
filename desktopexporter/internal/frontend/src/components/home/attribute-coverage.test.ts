import { afterEach, expect, it, vi } from 'vitest'
import { telemetryAPI } from '@/services/telemetry-service'
import type {
  AttributeFrequency,
  DiscoverySignal,
} from '@/services/discovery-types'
import {
  loadAttributeCoverage,
  mergeAttributeKeys,
  mergeAttributeValues,
  sortCoverageValues,
} from './attribute-coverage'

afterEach(() => vi.restoreAllMocks())

function frequency(
  value: string,
  count: number,
  denominator: number,
  signal: DiscoverySignal = 'traces'
): AttributeFrequency {
  return {
    identity: JSON.stringify({ kind: 'string', value }),
    value: { kind: 'string', value },
    count,
    kindCount: denominator,
    denominator,
    relativeFrequency: count / denominator,
    foundOn: [{ signal, ownerType: 'resource' }],
  }
}

it('weights shared resource values by the records carrying them, keeping native identities', () => {
  const coverage = mergeAttributeValues([
    {
      signal: 'traces',
      values: [frequency('prod', 9, 10), frequency('test', 1, 10)],
    },
    {
      signal: 'logs',
      values: [
        frequency('prod', 1, 90, 'logs'),
        frequency('test', 89, 90, 'logs'),
      ],
    },
  ])
  expect(coverage.denominator).toBe(100)
  expect(
    coverage.values.find(item => item.value.value === 'prod')
  ).toMatchObject({
    count: 10,
    relativeFrequency: 0.1,
    counts: { traces: 9, logs: 1, metrics: 0 },
  })
  expect(
    mergeAttributeKeys([
      {
        key: 'env',
        kind: 'string',
        ownerType: 'resource',
        foundOn: [{ signal: 'traces', ownerType: 'resource' }],
      },
      {
        key: 'env',
        kind: 'string',
        ownerType: 'resource',
        foundOn: [{ signal: 'logs', ownerType: 'resource' }],
      },
      {
        key: 'env',
        kind: 'string',
        ownerType: 'span',
        foundOn: [{ signal: 'traces', ownerType: 'span' }],
      },
      {
        key: 'env',
        kind: 'int64',
        ownerType: 'resource',
        foundOn: [{ signal: 'traces', ownerType: 'resource' }],
      },
    ])
  ).toHaveLength(3)
})

it('retains independent percentages when values share a record instead of normalizing them', () => {
  const coverage = mergeAttributeValues([
    {
      signal: 'traces',
      values: [frequency('first', 3, 4), frequency('second', 3, 4)],
    },
  ])
  expect(coverage.values.map(item => item.relativeFrequency)).toEqual([
    0.75, 0.75,
  ])
  expect(coverage.kinds.get('string')).toEqual({
    count: 4,
    counts: { traces: 4, logs: 0, metrics: 0 },
  })
})

it('pools backend kind counts once per signal, even when same-kind values overlap or differ between signals', () => {
  const integer = (
    count: number,
    denominator: number,
    signal: DiscoverySignal
  ): AttributeFrequency => ({
    ...frequency('1', count, denominator, signal),
    identity: '{"kind":"int64","value":"1"}',
    value: { kind: 'int64', value: 1n },
    kindCount: count,
  })
  const coverage = mergeAttributeValues([
    {
      signal: 'traces',
      values: [
        { ...frequency('first', 2, 3), kindCount: 2 },
        { ...frequency('second', 2, 3), kindCount: 2 },
        integer(1, 3, 'traces'),
      ],
    },
    {
      signal: 'logs',
      values: [
        { ...frequency('other', 9, 10, 'logs'), kindCount: 9 },
        integer(1, 10, 'logs'),
      ],
    },
    { signal: 'metrics', values: [integer(4, 4, 'metrics')] },
  ])
  expect(coverage.kinds.get('string')).toEqual({
    count: 11,
    counts: { traces: 2, logs: 9, metrics: 0 },
  })
  expect(coverage.kinds.get('int64')).toEqual({
    count: 6,
    counts: { traces: 1, logs: 1, metrics: 4 },
  })
  expect(coverage.denominator).toBe(17)
  expect(coverage.denominators).toEqual({ traces: 3, logs: 10, metrics: 4 })
  expect(
    coverage.values
      .filter(item => item.value.kind === 'string')
      .map(item => item.kindCount)
  ).toEqual([11, 11, 11])
  expect(
    coverage.values.find(item => item.value.kind === 'int64')
  ).toMatchObject({
    count: 6,
    kindCount: 6,
    denominator: 17,
    relativeFrequency: 6 / 17,
  })
  expect(coverage.values[0]).toMatchObject({
    count: 2,
    denominator: 17,
    relativeFrequency: 2 / 17,
  })
})

it('loads beyond the first page before cross-signal sorting and keeps all rare values', async () => {
  const first = Array.from({ length: 500 }, (_, index) =>
    frequency(`rare-${index}`, 1, 1001)
  )
  const read = vi
    .spyOn(telemetryAPI, 'discoverAttributeValues')
    .mockImplementation(async options => ({
      items:
        options.signal === 'traces'
          ? options.offset === 0
            ? first
            : [frequency('shared', 501, 1001)]
          : [frequency('shared', 99, 99, 'logs')],
      truncated: options.signal === 'traces' && options.offset === 0,
    }))
  const coverage = await loadAttributeCoverage(
    {
      ownerType: 'resource',
      key: 'env',
      queries: { traces: null, logs: null, metrics: false },
    },
    new AbortController().signal
  )
  expect(read).toHaveBeenCalledTimes(3)
  expect(coverage.values).toHaveLength(501)
  expect(coverage.kinds.get('string')).toEqual({
    count: 1100,
    counts: { traces: 1001, logs: 99, metrics: 0 },
  })
  expect(
    sortCoverageValues(coverage.values, 'frequency', 'desc')[0]
  ).toMatchObject({ count: 600, denominator: 1100 })
  expect(
    sortCoverageValues(coverage.values, 'frequency', 'asc').at(-1)?.value.value
  ).toBe('shared')
})

it('does not merge double bit patterns or equal-looking values with different types', () => {
  const base = frequency('0', 1, 4)
  const coverage = mergeAttributeValues([
    {
      signal: 'traces',
      values: [
        { ...base, kindCount: 1 },
        {
          ...base,
          identity: '{"kind":"int64","value":"0"}',
          value: { kind: 'int64', value: 0n },
          kindCount: 1,
        },
        {
          ...base,
          identity: '{"kind":"double","value":0}',
          value: { kind: 'double', value: 0 },
          kindCount: 2,
        },
        {
          ...base,
          identity: '{"kind":"double","value":"0x8000000000000000"}',
          value: { kind: 'double', value: -0 },
          kindCount: 2,
        },
      ],
    },
  ])
  expect(coverage.values).toHaveLength(4)
  expect(coverage.values.map(item => item.count)).toEqual([1, 1, 1, 1])
})
