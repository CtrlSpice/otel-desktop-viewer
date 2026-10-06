/**
 * Per-metric view state that survives navigation: which timeseries are
 * checked, which AggregationView the user last picked, and whether the
 * optional all-series aggregate line is shown.
 *
 * Stored as one JSON blob per Metric reference so the user's
 * "how I had this metric set up" travels together. Storage shape:
 *
 *   {
 *     visibleKeys?: string[],
 *     aggregationView?: AggregationView,
 *     showAllSeriesAggregate?: boolean
 *   }
 *
 * The one-shot empty-selection repair omits visibleKeys so normal defaults can
 * seed the chart without discarding the other settings. Other optional fields
 * are omitted from disk when undefined/false-default.
 */

import type { AggregationView } from './aggregation'

/** Maximum visible Gauge or Sum series. Histograms are uncapped. */
export const MAX_VISIBLE_TIMESERIES = 22

/**
 * How many timeseries to auto-select on first load (before the user
 * has made any explicit choices). Lower than MAX_VISIBLE_TIMESERIES
 * so the initial chart is readable; the user can manually check more
 * up to the cap.
 */
export const DEFAULT_VISIBLE_TIMESERIES = 10

const STORAGE_PREFIX = 'metrics:view:'

type PersistedMetricView = {
  visibleKeys?: string[]
  aggregationView?: AggregationView
  showAllSeriesAggregate?: boolean
}

type PersistedMetricViewFields = {
  visibleKeys?: unknown
  aggregationView?: unknown
  showAllSeriesAggregate?: unknown
}

function isPersistedMetricViewFields(
  value: unknown
): value is PersistedMetricViewFields {
  return value !== null && typeof value === 'object'
}

function isString(value: unknown): value is string {
  return typeof value === 'string'
}

function isAggregationView(value: unknown): value is AggregationView {
  return (
    value === 'raw' || value === 'sum' || value === 'avg' || value === 'rate'
  )
}

function isBoolean(value: unknown): value is boolean {
  return value === true || value === false
}

/**
 * Storage-format marker. Bumped when a stored view needs repairing rather than
 * merely reading differently.
 */
const STORAGE_VERSION_KEY = 'metrics:view:storage-version'
const STORAGE_VERSION = '1'

/**
 * One-time repair for empty visible-key lists stored before version 1. Later
 * empty lists are user choices. Other stored fields remain unchanged.
 */
export function repairEmptyPersistedVisibleKeys(): void {
  try {
    if (typeof localStorage === 'undefined') return
    if (localStorage.getItem(STORAGE_VERSION_KEY) === STORAGE_VERSION) return

    // Collect first because removing entries shifts later Storage indices.
    const storedKeys: string[] = []
    for (let i = 0; i < localStorage.length; i++) {
      const key = localStorage.key(i)
      if (key) storedKeys.push(key)
    }

    for (const key of storedKeys) {
      if (!key.startsWith(STORAGE_PREFIX)) continue
      if (key === STORAGE_VERSION_KEY) continue
      const raw = localStorage.getItem(key)
      if (!raw) continue
      let parsed: unknown
      try {
        parsed = JSON.parse(raw)
      } catch {
        continue
      }
      if (!isPersistedMetricViewFields(parsed)) continue
      if (!Array.isArray(parsed.visibleKeys) || parsed.visibleKeys.length > 0)
        continue

      delete parsed.visibleKeys
      if (Object.keys(parsed).length === 0) {
        localStorage.removeItem(key)
      } else {
        localStorage.setItem(key, JSON.stringify(parsed))
      }
    }

    localStorage.setItem(STORAGE_VERSION_KEY, STORAGE_VERSION)
  } catch {
    // Preference repair must not block page load when storage is unavailable.
  }
}

export function metricViewStorageKey(metricRef: string): string {
  return `${STORAGE_PREFIX}${metricRef}`
}

function loadPersistedView(metricRef: string): PersistedMetricView | null {
  try {
    if (typeof localStorage === 'undefined') return null
    const raw = localStorage.getItem(metricViewStorageKey(metricRef))
    if (!raw) return null
    const parsed: unknown = JSON.parse(raw)
    if (!isPersistedMetricViewFields(parsed)) return null
    const keysRaw = parsed.visibleKeys
    if (keysRaw !== undefined && !Array.isArray(keysRaw)) return null
    const visibleKeys = keysRaw?.filter(isString)
    const aggregationView = isAggregationView(parsed.aggregationView)
      ? parsed.aggregationView
      : undefined
    const showAllSeriesAggregate = isBoolean(parsed.showAllSeriesAggregate)
      ? parsed.showAllSeriesAggregate
      : undefined
    return {
      visibleKeys,
      aggregationView,
      showAllSeriesAggregate,
    }
  } catch {
    return null
  }
}

function serializePersistedView(view: PersistedMetricView): string {
  const payload: PersistedMetricView = {}
  if (view.visibleKeys !== undefined) {
    payload.visibleKeys = view.visibleKeys
  }
  if (view.aggregationView !== undefined) {
    payload.aggregationView = view.aggregationView
  }
  if (view.showAllSeriesAggregate === true) {
    payload.showAllSeriesAggregate = true
  }
  return JSON.stringify(payload)
}

function writePersistedView(
  metricRef: string,
  view: PersistedMetricView
): void {
  try {
    if (typeof localStorage === 'undefined') return
    localStorage.setItem(
      metricViewStorageKey(metricRef),
      serializePersistedView(view)
    )
  } catch {
    // A failed localStorage preference write must not abort the interaction.
  }
}

function mergePersistedView(
  existing: PersistedMetricView | null,
  patch: Partial<PersistedMetricView>
): PersistedMetricView {
  return {
    visibleKeys:
      'visibleKeys' in patch ? patch.visibleKeys : existing?.visibleKeys,
    aggregationView:
      'aggregationView' in patch
        ? patch.aggregationView
        : existing?.aggregationView,
    showAllSeriesAggregate:
      'showAllSeriesAggregate' in patch
        ? patch.showAllSeriesAggregate
        : existing?.showAllSeriesAggregate,
  }
}

/** Persist visible keys without changing other metric-view preferences. */
export function savePersistedTimeseriesVisible(
  metricRef: string,
  keys: Iterable<string>
): void {
  const existing = loadPersistedView(metricRef)
  writePersistedView(
    metricRef,
    mergePersistedView(existing, { visibleKeys: [...keys] })
  )
}

/** Persist the aggregation view without changing other metric-view preferences. */
export function savePersistedAggregationView(
  metricRef: string,
  aggregationView: AggregationView
): void {
  const existing = loadPersistedView(metricRef)
  writePersistedView(
    metricRef,
    mergePersistedView(existing, {
      aggregationView,
    })
  )
}

/** Persist whether the optional all-series aggregate line is shown. */
export function savePersistedShowAllSeriesAggregate(
  metricRef: string,
  showAllSeriesAggregate: boolean
): void {
  const existing = loadPersistedView(metricRef)
  writePersistedView(
    metricRef,
    mergePersistedView(existing, {
      showAllSeriesAggregate,
    })
  )
}

/** Read an allowed persisted aggregation view, or null. */
export function loadPersistedAggregationView(
  metricRef: string,
  allowed: readonly AggregationView[]
): AggregationView | null {
  const v = loadPersistedView(metricRef)?.aggregationView
  if (v === undefined) return null
  return allowed.includes(v) ? v : null
}

/** Read persisted all-series aggregate toggle. Defaults to false. */
export function loadPersistedShowAllSeriesAggregate(
  metricRef: string
): boolean {
  return loadPersistedView(metricRef)?.showAllSeriesAggregate === true
}

/**
 * Persisted visible keys, or null. This does not require the current metric
 * response, so callers can request the selected series in advance.
 */
export function persistedVisibleKeys(metricRef: string): string[] | null {
  return loadPersistedView(metricRef)?.visibleKeys ?? null
}

export function resolveTimeseriesVisible(
  currentKeys: readonly string[],
  metricRef: string,
  initialVisible: number = DEFAULT_VISIBLE_TIMESERIES,
  maxChecked: number | null = MAX_VISIBLE_TIMESERIES
): string[] {
  const persisted = loadPersistedView(metricRef)?.visibleKeys ?? null
  if (persisted !== null) {
    const current = new Set(currentKeys)
    const kept = persisted.filter(k => current.has(k))
    return maxChecked === null ? kept : kept.slice(0, maxChecked)
  }
  return currentKeys.slice(0, initialVisible)
}

/** Drop keys no longer present after a refresh; re-seed only when stale. */
export function reconcileTimeseriesVisible(
  visible: ReadonlySet<string>,
  currentKeys: readonly string[],
  metricRef: string,
  maxChecked: number | null = MAX_VISIBLE_TIMESERIES
): string[] {
  const current = new Set(currentKeys)
  const hadStale = [...visible].some(k => !current.has(k))
  const kept = [...visible].filter(k => current.has(k))
  const capped = maxChecked === null ? kept : kept.slice(0, maxChecked)
  if (capped.length > 0 || !hadStale) return capped
  return resolveTimeseriesVisible(
    currentKeys,
    metricRef,
    DEFAULT_VISIBLE_TIMESERIES,
    maxChecked
  )
}

export function visibleKeyListsEqual(
  a: Iterable<string>,
  b: readonly string[]
): boolean {
  const left = [...a].sort()
  const right = [...b].sort()
  return left.length === right.length && left.every((k, i) => k === right[i])
}
