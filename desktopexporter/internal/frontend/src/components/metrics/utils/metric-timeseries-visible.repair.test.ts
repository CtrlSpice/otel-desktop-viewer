// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  loadPersistedAggregationView,
  loadPersistedShowAllSeriesAggregate,
  metricViewStorageKey,
  persistedVisibleKeys,
  repairEmptyPersistedVisibleKeys,
  resolveTimeseriesVisible,
  savePersistedAggregationView,
  savePersistedShowAllSeriesAggregate,
  savePersistedTimeseriesVisible,
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

  afterEach(() => {
    vi.restoreAllMocks()
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
      JSON.stringify({
        visibleKeys: [],
        aggregationView: 'rate',
        showAllSeriesAggregate: true,
        futurePreference: 'preserved',
      })
    )
    repairEmptyPersistedVisibleKeys()
    const stored = JSON.parse(localStorage.getItem(metricViewStorageKey('m1'))!)
    expect(stored.aggregationView).toBe('rate')
    expect(stored.showAllSeriesAggregate).toBe(true)
    expect(stored.futurePreference).toBe('preserved')
    expect(stored.visibleKeys).toBeUndefined()
    expect(loadPersistedAggregationView('m1', ['raw', 'rate'])).toBe('rate')
    expect(loadPersistedShowAllSeriesAggregate('m1')).toBe(true)
    expect(resolveTimeseriesVisible(keys, 'm1')).toEqual(keys)
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

  it('leaves the migration pending when storage access is blocked', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('blocked')
    })
    expect(() => repairEmptyPersistedVisibleKeys()).not.toThrow()
    vi.restoreAllMocks()
    expect(localStorage.getItem('metrics:view:storage-version')).toBeNull()
  })

  it('treats a throwing storage getter as unavailable', () => {
    vi.spyOn(globalThis, 'localStorage', 'get').mockImplementation(() => {
      throw new Error('blocked')
    })

    expect(() => repairEmptyPersistedVisibleKeys()).not.toThrow()
    expect(() => savePersistedTimeseriesVisible('m1', ['a'])).not.toThrow()
    expect(() => savePersistedAggregationView('m1', 'sum')).not.toThrow()
    expect(() => savePersistedShowAllSeriesAggregate('m1', true)).not.toThrow()
    expect(persistedVisibleKeys('m1')).toBeNull()
    expect(loadPersistedAggregationView('m1', ['sum'])).toBeNull()
    expect(loadPersistedShowAllSeriesAggregate('m1')).toBe(false)
  })

  it('keeps saves non-fatal when writing storage fails', () => {
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('quota exceeded')
    })

    expect(() => savePersistedTimeseriesVisible('m1', ['a'])).not.toThrow()
    expect(() => savePersistedAggregationView('m1', 'sum')).not.toThrow()
    expect(() => savePersistedShowAllSeriesAggregate('m1', true)).not.toThrow()
  })

  it('does not recreate an empty selection when settings change after repair', () => {
    localStorage.setItem(
      metricViewStorageKey('m5'),
      JSON.stringify({
        visibleKeys: [],
        aggregationView: 'rate',
        showAllSeriesAggregate: true,
      })
    )
    repairEmptyPersistedVisibleKeys()

    savePersistedAggregationView('m5', 'sum')
    savePersistedShowAllSeriesAggregate('m5', false)

    const stored = JSON.parse(localStorage.getItem(metricViewStorageKey('m5'))!)
    expect(stored).toEqual({ aggregationView: 'sum' })
    expect(resolveTimeseriesVisible(keys, 'm5')).toEqual(keys)
  })
})

describe('persisted metric view decoding', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('salvages valid fields and visible keys without changing their order', () => {
    localStorage.setItem(
      metricViewStorageKey('mixed'),
      JSON.stringify({
        visibleKeys: ['b', 7, 'a', null, 'b'],
        aggregationView: 'rate',
        showAllSeriesAggregate: true,
      })
    )

    expect(persistedVisibleKeys('mixed')).toEqual(['b', 'a', 'b'])
    expect(loadPersistedAggregationView('mixed', ['raw', 'rate'])).toBe('rate')
    expect(loadPersistedShowAllSeriesAggregate('mixed')).toBe(true)
  })

  it('rejects the whole view when visibleKeys is not an array', () => {
    localStorage.setItem(
      metricViewStorageKey('invalid-list'),
      JSON.stringify({
        visibleKeys: 'a',
        aggregationView: 'rate',
        showAllSeriesAggregate: true,
      })
    )

    expect(persistedVisibleKeys('invalid-list')).toBeNull()
    expect(loadPersistedAggregationView('invalid-list', ['rate'])).toBeNull()
    expect(loadPersistedShowAllSeriesAggregate('invalid-list')).toBe(false)
  })

  it('ignores malformed optional fields while keeping visible keys', () => {
    localStorage.setItem(
      metricViewStorageKey('invalid-options'),
      JSON.stringify({
        visibleKeys: ['a'],
        aggregationView: 'total',
        showAllSeriesAggregate: 'yes',
      })
    )

    expect(persistedVisibleKeys('invalid-options')).toEqual(['a'])
    expect(loadPersistedAggregationView('invalid-options', ['raw'])).toBeNull()
    expect(loadPersistedShowAllSeriesAggregate('invalid-options')).toBe(false)
  })

  it('rewrites a partially valid view canonically on the next save', () => {
    localStorage.setItem(
      metricViewStorageKey('rewrite'),
      JSON.stringify({
        visibleKeys: ['b', 7, 'a'],
        aggregationView: 'invalid',
        showAllSeriesAggregate: true,
      })
    )

    savePersistedAggregationView('rewrite', 'sum')
    expect(localStorage.getItem(metricViewStorageKey('rewrite'))).toBe(
      JSON.stringify({
        visibleKeys: ['b', 'a'],
        aggregationView: 'sum',
        showAllSeriesAggregate: true,
      })
    )
  })

  it('preserves a deliberate empty selection after migration', () => {
    repairEmptyPersistedVisibleKeys()
    savePersistedTimeseriesVisible('empty', [])
    savePersistedAggregationView('empty', 'sum')
    repairEmptyPersistedVisibleKeys()
    expect(persistedVisibleKeys('empty')).toEqual([])
    expect(loadPersistedAggregationView('empty', ['sum'])).toBe('sum')
  })
})
