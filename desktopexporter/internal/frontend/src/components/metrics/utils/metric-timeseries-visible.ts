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

const STORAGE_PREFIX = 'metrics:timeseries-visible:'

/** Same identity as `metricSummaryKey` / drawer search — metric stream id. */
export function metricTimeseriesVisibleStorageKey(metricStreamId: string): string {
  return `${STORAGE_PREFIX}${metricStreamId}`
}

export function loadPersistedTimeseriesVisible(
  metricStreamId: string
): string[] | null {
  if (typeof localStorage === 'undefined') return null
  try {
    const raw = localStorage.getItem(
      metricTimeseriesVisibleStorageKey(metricStreamId)
    )
    if (!raw) return null
    const parsed: unknown = JSON.parse(raw)
    if (!Array.isArray(parsed)) return null
    return parsed.filter((k): k is string => typeof k === 'string')
  } catch {
    return null
  }
}

export function savePersistedTimeseriesVisible(
  metricStreamId: string,
  keys: Iterable<string>
): void {
  if (typeof localStorage === 'undefined') return
  localStorage.setItem(
    metricTimeseriesVisibleStorageKey(metricStreamId),
    JSON.stringify([...keys])
  )
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
  const persisted = loadPersistedTimeseriesVisible(metricStreamId)
  if (persisted !== null) {
    const current = new Set(currentKeys)
    const kept = persisted.filter((k) => current.has(k))
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
  const hadStale = [...visible].some((k) => !current.has(k))
  const kept = [...visible].filter((k) => current.has(k))
  const capped =
    maxChecked === null ? kept : kept.slice(0, maxChecked)
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
