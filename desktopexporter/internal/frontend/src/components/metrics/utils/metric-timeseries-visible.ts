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
 *
 * 22 is a full F1 grid: 11 teams × 2 cars.
 *
 * Note this is sized for a view that does not exist yet. Today no single
 * stream comes near it: metric_streams identity includes service_name and
 * each team is its own service, so a per-driver metric is 11 separate
 * streams of 2 series rather than one stream of 22. Raising the cap
 * changes nothing observable until those streams merge.
 */
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
/**
 * Storage-format marker. Bumped when a stored view needs repairing rather than
 * merely reading differently.
 */
const STORAGE_VERSION_KEY = 'metrics:view:storage-version'
const STORAGE_VERSION = '1'

/**
 * Drop empty visible-key lists written by a bug, once.
 *
 * Per-metric view state used to be seeded in an $effect, which ran after the
 * first render and could therefore run before the series keys had settled. It
 * would then persist `visibleKeys: []` for a metric the user had never touched,
 * and every later visit honoured it: a chart with nothing drawn and every
 * series unticked. Seeding is synchronous now and cannot produce it.
 *
 * An empty list cannot be told from a deliberate one by inspection -- unticking
 * every series is a legitimate thing to do and must survive a reload. So this
 * runs exactly once, guarded by a version marker: entries emptied by the old
 * code are cleared, and anything a user empties afterwards is left alone
 * forever.
 *
 * Only `visibleKeys` is removed; the rest of the stored view (aggregation view,
 * aggregate toggles) is a separate choice and is kept.
 */
export function repairEmptyPersistedVisibleKeys(): void {
  if (typeof localStorage === 'undefined') return
  try {
    if (localStorage.getItem(STORAGE_VERSION_KEY) === STORAGE_VERSION) return

    // Collected first, and read through the Storage API rather than
    // Object.keys: enumerating a Storage as a plain object is a browser
    // convenience, not part of the interface, and removing while iterating
    // shifts every later index.
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
      if (!parsed || typeof parsed !== 'object') continue
      const obj = parsed as Record<string, unknown>
      if (!Array.isArray(obj.visibleKeys) || obj.visibleKeys.length > 0)
        continue

      delete obj.visibleKeys
      if (Object.keys(obj).length === 0) {
        localStorage.removeItem(key)
      } else {
        localStorage.setItem(key, JSON.stringify(obj))
      }
    }

    localStorage.setItem(STORAGE_VERSION_KEY, STORAGE_VERSION)
  } catch {
    // A storage that throws (private mode, quota) is not worth failing a page
    // load over. The unrepaired state is the status quo, not a new fault.
  }
}

export function metricViewStorageKey(metricStreamID: string): string {
  return `${STORAGE_PREFIX}${metricStreamID}`
}

function loadPersistedView(metricStreamID: string): PersistedMetricView | null {
  if (typeof localStorage === 'undefined') return null
  try {
    const raw = localStorage.getItem(metricViewStorageKey(metricStreamID))
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
  metricStreamID: string,
  view: PersistedMetricView
): void {
  if (typeof localStorage === 'undefined') return
  localStorage.setItem(
    metricViewStorageKey(metricStreamID),
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
  metricStreamID: string,
  keys: Iterable<string>
): void {
  const existing = loadPersistedView(metricStreamID)
  writePersistedView(
    metricStreamID,
    mergePersistedView(existing, { visibleKeys: [...keys] })
  )
}

/**
 * Persist the aggregationView, preserving the existing visibleKeys on
 * disk. Mirror of {@link savePersistedTimeseriesVisible} — same read-
 * modify-write discipline so the two writers don't fight.
 */
export function savePersistedAggregationView(
  metricStreamID: string,
  aggregationView: AggregationView
): void {
  const existing = loadPersistedView(metricStreamID)
  writePersistedView(
    metricStreamID,
    mergePersistedView(existing, {
      visibleKeys: existing?.visibleKeys ?? [],
      aggregationView,
    })
  )
}

/** Persist whether the optional all-series aggregate line is shown. */
export function savePersistedShowAllSeriesAggregate(
  metricStreamID: string,
  showAllSeriesAggregate: boolean
): void {
  const existing = loadPersistedView(metricStreamID)
  writePersistedView(
    metricStreamID,
    mergePersistedView(existing, {
      visibleKeys: existing?.visibleKeys ?? [],
      showAllSeriesAggregate,
    })
  )
}

/** Persist whether the optional all-series quantile lines are shown. */
export function savePersistedShowAllSeriesQuantileAggregate(
  metricStreamID: string,
  showAllSeriesQuantileAggregate: boolean
): void {
  const existing = loadPersistedView(metricStreamID)
  writePersistedView(
    metricStreamID,
    mergePersistedView(existing, {
      visibleKeys: existing?.visibleKeys ?? [],
      showAllSeriesQuantileAggregate,
    })
  )
}

export function loadPersistedShowAllSeriesQuantileAggregate(
  metricStreamID: string
): boolean {
  return (
    loadPersistedView(metricStreamID)?.showAllSeriesQuantileAggregate === true
  )
}

/**
 * Read the persisted aggregationView. Returns null when there is no
 * entry, the entry is invalid, or the persisted value isn't in
 * `allowed` (e.g. user previously picked Sum on a metric that's now
 * 1-series). Caller decides the fallback.
 */
export function loadPersistedAggregationView(
  metricStreamID: string,
  allowed: readonly AggregationView[]
): AggregationView | null {
  const v = loadPersistedView(metricStreamID)?.aggregationView
  if (v === undefined) return null
  return allowed.includes(v) ? v : null
}

/** Read persisted all-series aggregate toggle. Defaults to false. */
export function loadPersistedShowAllSeriesAggregate(
  metricStreamID: string
): boolean {
  return loadPersistedView(metricStreamID)?.showAllSeriesAggregate === true
}

/**
 * Pick visible timeseries keys for the current metric data.
 * Restores persisted keys that still exist; otherwise first N.
 */
/**
 * The visible keys a previous visit left behind, or null if there are none.
 *
 * Exposed so a fetch can name the series it needs before it has the response:
 * resolveTimeseriesVisible answers the same question but needs the metric's
 * current keys to fall back on, and that is exactly what the caller does not
 * have yet.
 */
export function persistedVisibleKeys(metricStreamID: string): string[] | null {
  return loadPersistedView(metricStreamID)?.visibleKeys ?? null
}

export function resolveTimeseriesVisible(
  currentKeys: readonly string[],
  metricStreamID: string,
  initialVisible: number = DEFAULT_VISIBLE_TIMESERIES,
  maxChecked: number | null = MAX_VISIBLE_TIMESERIES
): string[] {
  const persisted = loadPersistedView(metricStreamID)?.visibleKeys ?? null
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
  metricStreamID: string,
  maxChecked: number | null = MAX_VISIBLE_TIMESERIES
): string[] {
  const current = new Set(currentKeys)
  const hadStale = [...visible].some(k => !current.has(k))
  const kept = [...visible].filter(k => current.has(k))
  const capped = maxChecked === null ? kept : kept.slice(0, maxChecked)
  if (capped.length > 0 || !hadStale) return capped
  return resolveTimeseriesVisible(
    currentKeys,
    metricStreamID,
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
