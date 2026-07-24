/**
 * Per-metric view state that survives navigation: which timeseries are
 * checked, which AggregationView the user last picked, and whether the
 * optional all-series aggregate line is shown.
 *
 * Stored as a single JSON blob per metric stream id so the user's
 * "how I had this metric set up" travels together. Storage shape:
 *
 *   {
 *     visibleKeys: string[],
 *     aggregationView?: AggregationView,
 *     showAllSeriesAggregate?: boolean
 *     showAllSeriesQuantileAggregate?: boolean
 *   }
 *
 * Optional fields are omitted from disk when undefined/false-default.
 */

import type { AggregationView } from './aggregation'

/**
 * Maximum number of timeseries that can be visible (checked) at once
 * for gauge/sum line charts. The legend disables further checkboxes
 * once this many are selected. Histogram metrics are uncapped — the
 * heatmap can show every attribute breakdown at once.
 */
export const MAX_VISIBLE_TIMESERIES = 10

/**
 * How many timeseries to auto-select on first load (before the user
 * has made any explicit choices). Lower than MAX_VISIBLE_TIMESERIES
 * so the initial chart is readable; the user can manually check more
 * up to the cap.
 */
export const DEFAULT_VISIBLE_TIMESERIES = 5

const STORAGE_PREFIX = 'metrics:view:'

type PersistedMetricView = {
  visibleKeys: string[]
  aggregationView?: AggregationView
  showAllSeriesAggregate?: boolean
  showAllSeriesQuantileAggregate?: boolean
}

const VALID_AGGREGATION_VIEWS: ReadonlySet<AggregationView> = new Set([
  'raw',
  'sum',
  'avg',
  'rate',
])

/** Same identity as `metricSummaryKey` / drawer search — metric stream id. */
export function metricViewStorageKey(metricStreamId: string): string {
  return `${STORAGE_PREFIX}${metricStreamId}`
}

function loadPersistedView(metricStreamId: string): PersistedMetricView | null {
  if (typeof localStorage === 'undefined') return null
  try {
    const raw = localStorage.getItem(metricViewStorageKey(metricStreamId))
    if (!raw) return null
    const parsed: unknown = JSON.parse(raw)
    if (!parsed || typeof parsed !== 'object') return null
    const obj = parsed as Record<string, unknown>
    const keysRaw = obj.visibleKeys
    if (!Array.isArray(keysRaw)) return null
    const visibleKeys = keysRaw.filter(
      (k): k is string => typeof k === 'string'
    )
    const av = obj.aggregationView
    const aggregationView =
      typeof av === 'string' &&
      VALID_AGGREGATION_VIEWS.has(av as AggregationView)
        ? (av as AggregationView)
        : undefined
    const sa = obj.showAllSeriesAggregate
    const showAllSeriesAggregate = typeof sa === 'boolean' ? sa : undefined
    const sq = obj.showAllSeriesQuantileAggregate
    const showAllSeriesQuantileAggregate =
      typeof sq === 'boolean' ? sq : undefined
    return {
      visibleKeys,
      aggregationView,
      showAllSeriesAggregate,
      showAllSeriesQuantileAggregate,
    }
  } catch {
    return null
  }
}

function serializePersistedView(view: PersistedMetricView): string {
  const payload: PersistedMetricView = { visibleKeys: view.visibleKeys }
  if (view.aggregationView !== undefined) {
    payload.aggregationView = view.aggregationView
  }
  if (view.showAllSeriesAggregate === true) {
    payload.showAllSeriesAggregate = true
  }
  if (view.showAllSeriesQuantileAggregate === true) {
    payload.showAllSeriesQuantileAggregate = true
  }
  return JSON.stringify(payload)
}

function writePersistedView(
  metricStreamId: string,
  view: PersistedMetricView
): void {
  if (typeof localStorage === 'undefined') return
  localStorage.setItem(
    metricViewStorageKey(metricStreamId),
    serializePersistedView(view)
  )
}

function mergePersistedView(
  existing: PersistedMetricView | null,
  patch:
    | (Partial<PersistedMetricView> & Pick<PersistedMetricView, 'visibleKeys'>)
    | { visibleKeys?: string[] }
): PersistedMetricView {
  return {
    visibleKeys: patch.visibleKeys ?? existing?.visibleKeys ?? [],
    aggregationView:
      'aggregationView' in patch
        ? patch.aggregationView
        : existing?.aggregationView,
    showAllSeriesAggregate:
      'showAllSeriesAggregate' in patch
        ? patch.showAllSeriesAggregate
        : existing?.showAllSeriesAggregate,
    showAllSeriesQuantileAggregate:
      'showAllSeriesQuantileAggregate' in patch
        ? patch.showAllSeriesQuantileAggregate
        : existing?.showAllSeriesQuantileAggregate,
  }
}

/**
 * Persist the visible-keys list, preserving any existing aggregationView
 * on disk. Call this from checkbox toggles; aggregationView is owned by
 * a different write path and must not be clobbered here.
 */
export function savePersistedTimeseriesVisible(
  metricStreamId: string,
  keys: Iterable<string>
): void {
  const existing = loadPersistedView(metricStreamId)
  writePersistedView(
    metricStreamId,
    mergePersistedView(existing, { visibleKeys: [...keys] })
  )
}

/**
 * Persist the aggregationView, preserving the existing visibleKeys on
 * disk. Mirror of {@link savePersistedTimeseriesVisible} — same read-
 * modify-write discipline so the two writers don't fight.
 */
export function savePersistedAggregationView(
  metricStreamId: string,
  aggregationView: AggregationView
): void {
  const existing = loadPersistedView(metricStreamId)
  writePersistedView(
    metricStreamId,
    mergePersistedView(existing, {
      visibleKeys: existing?.visibleKeys ?? [],
      aggregationView,
    })
  )
}

/** Persist whether the optional all-series aggregate line is shown. */
export function savePersistedShowAllSeriesAggregate(
  metricStreamId: string,
  showAllSeriesAggregate: boolean
): void {
  const existing = loadPersistedView(metricStreamId)
  writePersistedView(
    metricStreamId,
    mergePersistedView(existing, {
      visibleKeys: existing?.visibleKeys ?? [],
      showAllSeriesAggregate,
    })
  )
}

/** Persist whether the optional all-series quantile lines are shown. */
export function savePersistedShowAllSeriesQuantileAggregate(
  metricStreamId: string,
  showAllSeriesQuantileAggregate: boolean
): void {
  const existing = loadPersistedView(metricStreamId)
  writePersistedView(
    metricStreamId,
    mergePersistedView(existing, {
      visibleKeys: existing?.visibleKeys ?? [],
      showAllSeriesQuantileAggregate,
    })
  )
}

export function loadPersistedShowAllSeriesQuantileAggregate(
  metricStreamId: string
): boolean {
  return (
    loadPersistedView(metricStreamId)?.showAllSeriesQuantileAggregate === true
  )
}

/**
 * Read the persisted aggregationView. Returns null when there is no
 * entry, the entry is invalid, or the persisted value isn't in
 * `allowed` (e.g. user previously picked Sum on a metric that's now
 * 1-series). Caller decides the fallback.
 */
export function loadPersistedAggregationView(
  metricStreamId: string,
  allowed: readonly AggregationView[]
): AggregationView | null {
  const v = loadPersistedView(metricStreamId)?.aggregationView
  if (v === undefined) return null
  return allowed.includes(v) ? v : null
}

/** Read persisted all-series aggregate toggle. Defaults to false. */
export function loadPersistedShowAllSeriesAggregate(
  metricStreamId: string
): boolean {
  return loadPersistedView(metricStreamId)?.showAllSeriesAggregate === true
}

/**
 * Pick visible timeseries keys for the current metric data.
 * Restores persisted keys that still exist; otherwise first N.
 */
export function resolveTimeseriesVisible(
  currentKeys: readonly string[],
  metricStreamId: string,
  initialVisible: number = DEFAULT_VISIBLE_TIMESERIES,
  maxChecked: number | null = MAX_VISIBLE_TIMESERIES
): string[] {
  const persisted = loadPersistedView(metricStreamId)?.visibleKeys ?? null
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
  metricStreamId: string,
  maxChecked: number | null = MAX_VISIBLE_TIMESERIES
): string[] {
  const current = new Set(currentKeys)
  const hadStale = [...visible].some(k => !current.has(k))
  const kept = [...visible].filter(k => current.has(k))
  const capped = maxChecked === null ? kept : kept.slice(0, maxChecked)
  if (capped.length > 0 || !hadStale) return capped
  return resolveTimeseriesVisible(
    currentKeys,
    metricStreamId,
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
