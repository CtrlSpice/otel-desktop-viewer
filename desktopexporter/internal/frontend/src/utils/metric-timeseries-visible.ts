import {
  DEFAULT_VISIBLE_TIMESERIES,
  MAX_VISIBLE_TIMESERIES,
} from '@/utils/timeseries-palette'

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
  maxVisible: number = DEFAULT_VISIBLE_TIMESERIES
): string[] {
  const persisted = loadPersistedTimeseriesVisible(metricStreamId)
  if (persisted !== null) {
    const current = new Set(currentKeys)
    return persisted
      .filter((k) => current.has(k))
      .slice(0, MAX_VISIBLE_TIMESERIES)
  }
  return currentKeys.slice(0, maxVisible)
}

/** Drop keys no longer present after a refresh; re-seed only when stale. */
export function reconcileTimeseriesVisible(
  visible: ReadonlySet<string>,
  currentKeys: readonly string[],
  metricStreamId: string,
): string[] {
  const current = new Set(currentKeys)
  const hadStale = [...visible].some((k) => !current.has(k))
  const kept = [...visible]
    .filter((k) => current.has(k))
    .slice(0, MAX_VISIBLE_TIMESERIES)
  if (kept.length > 0 || !hadStale) return kept
  return resolveTimeseriesVisible(currentKeys, metricStreamId)
}

export function visibleKeyListsEqual(
  a: Iterable<string>,
  b: readonly string[]
): boolean {
  const left = [...a].sort()
  const right = [...b].sort()
  return left.length === right.length && left.every((k, i) => k === right[i])
}
