// @vitest-environment jsdom
import { beforeEach, describe, expect, it } from 'vitest'
import {
  metricViewStorageKey,
  repairEmptyPersistedVisibleKeys,
  resolveTimeseriesVisible,
} from '@/components/metrics/utils/metric-timeseries-visible'

/*
 * The repair exists because the old $effect-based seeding could persist
 * `visibleKeys: []` for a metric nobody had touched -- it ran after the first
 * render, so it could write before the series keys had settled. Every later
 * visit then honoured it and drew nothing.
 *
 * An empty list written on purpose is indistinguishable from one written by
 * that bug, so the repair is one-shot: it clears what is there now, and never
 * looks again.
 */
describe('repairEmptyPersistedVisibleKeys', () => {
  const keys = ['a', 'b', 'c']

  beforeEach(() => {
    localStorage.clear()
  })

  it('clears an empty list so the metric falls back to the default selection', () => {
    localStorage.setItem(
      metricViewStorageKey('m1'),
      JSON.stringify({ visibleKeys: [], aggregationView: 'rate' })
    )

    expect(resolveTimeseriesVisible(keys, 'm1')).toEqual([])
    repairEmptyPersistedVisibleKeys()
    expect(resolveTimeseriesVisible(keys, 'm1')).toEqual(keys)
  })

  it('keeps the rest of the stored view', () => {
    localStorage.setItem(
      metricViewStorageKey('m1'),
      JSON.stringify({ visibleKeys: [], aggregationView: 'rate' })
    )
    repairEmptyPersistedVisibleKeys()
    const stored = JSON.parse(localStorage.getItem(metricViewStorageKey('m1'))!)
    expect(stored.aggregationView).toBe('rate')
    expect(stored.visibleKeys).toBeUndefined()
  })

  it('leaves a non-empty selection alone', () => {
    localStorage.setItem(
      metricViewStorageKey('m2'),
      JSON.stringify({ visibleKeys: ['b'] })
    )
    repairEmptyPersistedVisibleKeys()
    expect(resolveTimeseriesVisible(keys, 'm2')).toEqual(['b'])
  })

  it('never touches an empty selection made after it ran', () => {
    repairEmptyPersistedVisibleKeys()
    // The user unticks everything, which is a real choice and must survive.
    localStorage.setItem(
      metricViewStorageKey('m3'),
      JSON.stringify({ visibleKeys: [] })
    )
    repairEmptyPersistedVisibleKeys()
    expect(resolveTimeseriesVisible(keys, 'm3')).toEqual([])
  })

  it('drops an entry that held nothing but the empty list', () => {
    localStorage.setItem(
      metricViewStorageKey('m4'),
      JSON.stringify({ visibleKeys: [] })
    )
    repairEmptyPersistedVisibleKeys()
    expect(localStorage.getItem(metricViewStorageKey('m4'))).toBeNull()
  })
})
