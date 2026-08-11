import { describe, it, expect } from 'vitest'
import { distinguishingResourceAttributes } from './series-labels'
import type { MetricTimeseries } from '@/types/api-types'

const series = (
  key: string,
  resourceAttrs: Array<[string, string]>
): MetricTimeseries =>
  ({
    attributesKey: key,
    attributes: [{ key: 'http.route', value: '/checkout', type: 'string' }],
    resource: {
      attributes: resourceAttrs.map(([k, v]) => ({
        key: k,
        value: v,
        type: 'string',
      })),
      droppedAttributesCount: 0,
    },
    datapoints: [],
  }) as unknown as MetricTimeseries

describe('distinguishingResourceAttributes', () => {
  // The case the whole feature exists for: two replicas of one service emit
  // byte-identical labels, so only the resource tells them apart.
  it('returns the attribute that varies between replicas', () => {
    const got = distinguishingResourceAttributes([
      series('a', [
        ['service.name', 'checkout'],
        ['host.name', 'pod-a'],
      ]),
      series('b', [
        ['service.name', 'checkout'],
        ['host.name', 'pod-b'],
      ]),
    ])
    expect(got.get('a')).toEqual([
      { key: 'host.name', value: 'pod-a', type: 'string' },
    ])
    expect(got.get('b')).toEqual([
      { key: 'host.name', value: 'pod-b', type: 'string' },
    ])
  })

  // The common case. A metric whose series share a resource must not gain a
  // column of identical values.
  it('returns nothing when every series shares a resource', () => {
    const got = distinguishingResourceAttributes([
      series('a', [
        ['service.name', 'checkout'],
        ['host.name', 'pod-a'],
      ]),
      series('b', [
        ['service.name', 'checkout'],
        ['host.name', 'pod-a'],
      ]),
    ])
    expect(got.get('a')).toEqual([])
    expect(got.get('b')).toEqual([])
  })

  it('adds nothing for a single series', () => {
    const got = distinguishingResourceAttributes([
      series('only', [['host.name', 'pod-a']]),
    ])
    expect(got.get('only')).toEqual([])
  })

  // A whole resource is ~15 attributes; surfacing all of them would bury the
  // one that matters.
  it('includes only the varying attributes, not the whole resource', () => {
    const shared: Array<[string, string]> = [
      ['service.name', 'checkout'],
      ['cloud.region', 'us-east-1'],
      ['os.type', 'linux'],
    ]
    const got = distinguishingResourceAttributes([
      series('a', [...shared, ['host.name', 'pod-a']]),
      series('b', [...shared, ['host.name', 'pod-b']]),
    ])
    expect(got.get('a')!.map(a => a.key)).toEqual(['host.name'])
  })

  // Absence is distinguishing too: a key one series has and another lacks
  // tells them apart just as well as differing values.
  it('treats a key missing from one series as distinguishing', () => {
    const got = distinguishingResourceAttributes([
      series('a', [
        ['service.name', 'checkout'],
        ['k8s.pod.name', 'p1'],
      ]),
      series('b', [['service.name', 'checkout']]),
    ])
    expect(got.get('a')!.map(a => a.key)).toEqual(['k8s.pod.name'])
    expect(got.get('b')).toEqual([])
  })

  it('survives a series with no resource attributes', () => {
    const got = distinguishingResourceAttributes([
      series('a', []),
      series('b', [['host.name', 'pod-b']]),
    ])
    expect(got.get('a')).toEqual([])
    expect(got.get('b')!.map(a => a.key)).toEqual(['host.name'])
  })
})
