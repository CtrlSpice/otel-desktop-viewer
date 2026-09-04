import { setContext, getContext } from 'svelte'
import {
  type Timezone,
  normalizeTimezone,
  recordRecentTimeRange,
} from '@/utils/time'
import {
  navigateCurrentRoute,
  readRoute,
  subscribeToRoute,
  withQueryPatch,
} from '@/route'

type TimeSelection =
  | { type: 'all' }
  | { type: 'preset'; presetIndex: number; durationMs: number }
  | { type: 'custom' | 'recent'; start: number; end: number }

export type QueryTimeRangeMs = {
  startTime: number | null
  endTime: number | null
}

/**
 * Unix ms range for search/export APIs.
 * Presets store their duration and are anchored so the window ends at `nowMs`.
 * Custom and recent use the stored bounds as-is.
 */
export function selectionToQueryRangeMs(
  selection: TimeSelection,
  nowMs: number
): QueryTimeRangeMs {
  if (selection.type === 'all') {
    return { startTime: null, endTime: null }
  }
  if (selection.type === 'preset') {
    return { startTime: nowMs - selection.durationMs, endTime: nowMs }
  }
  return { startTime: selection.start, endTime: selection.end }
}

interface TimeContext {
  selection: TimeSelection
  tz: Timezone
  setSelection: (selection: TimeSelection) => void
  setTz: (tz: Timezone) => void
}

function isFiniteNumber(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value)
}

function hasOnlyKeys(
  value: Record<string, unknown>,
  keys: readonly string[]
): boolean {
  const actual = Object.keys(value)
  return actual.length === keys.length && keys.every(key => key in value)
}

function isBoundedSelection(
  value: unknown
): value is Extract<TimeSelection, { type: 'custom' | 'recent' }> {
  if (!value || typeof value !== 'object') return false
  const candidate = value as Record<string, unknown>
  return (
    (candidate.type === 'custom' || candidate.type === 'recent') &&
    hasOnlyKeys(candidate, ['type', 'start', 'end']) &&
    isFiniteNumber(candidate.start) &&
    isFiniteNumber(candidate.end) &&
    candidate.start < candidate.end
  )
}

function isTimeSelection(value: unknown): value is TimeSelection {
  if (!value || typeof value !== 'object') return false
  const candidate = value as Record<string, unknown>
  if (candidate.type === 'all') return hasOnlyKeys(candidate, ['type'])
  if (isBoundedSelection(candidate)) return true
  return (
    candidate.type === 'preset' &&
    hasOnlyKeys(candidate, ['type', 'presetIndex', 'durationMs']) &&
    Number.isInteger(candidate.presetIndex) &&
    Number(candidate.presetIndex) > 0 &&
    isFiniteNumber(candidate.durationMs) &&
    candidate.durationMs > 0
  )
}

/** Restore a current persisted shape, otherwise use the unbounded default. */
function loadTimeSelection(raw: string | null): TimeSelection {
  if (!raw) return { type: 'all' }

  try {
    const parsed: unknown = JSON.parse(raw)
    return isTimeSelection(parsed) ? parsed : { type: 'all' }
  } catch {
    return { type: 'all' }
  }
}

function parseTimezone(value: string | null): Timezone | null {
  return value ? normalizeTimezone(value) : null
}

type RouteTimeSnapshot =
  { type: 'all' } | { type: 'bounded'; start: number; end: number }

/** Parse explicit All or a bounded `start`/`end` pair from the route. */
function parseTimeQuery(
  query: Record<string, string>
): RouteTimeSnapshot | null {
  if (query.time === 'all') return { type: 'all' }
  const start = Number(query.start)
  const end = Number(query.end)
  if (
    !query.start ||
    !query.end ||
    !Number.isFinite(start) ||
    !Number.isFinite(end) ||
    start >= end
  ) {
    return null
  }
  return { type: 'bounded', start, end }
}

function sameRouteTime(
  left: RouteTimeSnapshot | null,
  right: RouteTimeSnapshot | null
): boolean {
  if (left?.type !== right?.type) return false
  if (!left || !right || left.type === 'all' || right.type === 'all') {
    return left?.type === right?.type
  }
  return left.start === right.start && left.end === right.end
}

/**
 * Read/write localStorage; hold reactive selection + tz.
 *
 * The active window is also mirrored to the URL so a link shared alongside the
 * DuckDB snapshot reopens the same range. Precedence on load is URL > localStorage
 * > default. The URL is only written when the user changes the window (not on
 * load), so users who never touch the picker keep their live localStorage preset.
 */
function createTimeContext(): TimeContext {
  const savedSelection = localStorage.getItem('time-selection')
  const savedTz = localStorage.getItem('time-tz')
  const restoredTz = parseTimezone(savedTz)
  if (savedTz && !restoredTz) localStorage.removeItem('time-tz')
  else if (restoredTz && restoredTz !== savedTz) {
    localStorage.setItem('time-tz', restoredTz)
  }

  const urlTime = parseTimeQuery(readRoute().query)

  let selection = $state<TimeSelection>(
    urlTime?.type === 'all'
      ? { type: 'all' }
      : urlTime?.type === 'bounded'
        ? { type: 'custom', start: urlTime.start, end: urlTime.end }
        : loadTimeSelection(savedSelection)
  )
  let tz = $state<Timezone>(restoredTz ?? 'local')

  // The absolute window currently frozen in the URL. Presets stay live in
  // memory (duration anchored to now) while the URL holds this fixed
  // start/end snapshot, so the two legitimately disagree — the router
  // subscription compares against this, not the live selection, to tell
  // external changes (back/forward, shared links) from our own writes.
  let urlWindowSnapshot: RouteTimeSnapshot | null = urlTime

  function syncUrl() {
    const range = selectionToQueryRangeMs(selection, Date.now())
    urlWindowSnapshot =
      range.startTime === null || range.endTime === null
        ? { type: 'all' }
        : { type: 'bounded', start: range.startTime, end: range.endTime }
    navigateCurrentRoute(
      withQueryPatch(readRoute().query, {
        time: selection.type === 'all' ? 'all' : null,
        start: range.startTime === null ? null : String(range.startTime),
        end: range.endTime === null ? null : String(range.endTime),
      }),
      'replace'
    )
  }

  function setSelection(next: TimeSelection) {
    const now = Date.now()
    selection = next
    localStorage.setItem('time-selection', JSON.stringify(selection))
    const range = selectionToQueryRangeMs(selection, now)
    if (
      (selection.type === 'custom' || selection.type === 'recent') &&
      range.startTime !== null &&
      range.endTime !== null
    ) {
      recordRecentTimeRange(range.startTime, range.endTime, now)
    }
    syncUrl()
  }

  function setTz(newTz: Timezone) {
    tz = newTz
    localStorage.setItem('time-tz', newTz)
  }

  // Adopt the window from the URL on external changes only. An external change
  // is one whose absolute bounds differ from what we last wrote, so item
  // navigation (which leaves the time query untouched) and our own writes are
  // both ignored — no feedback loop, no clobbering live presets.
  $effect(() => {
    const unsubscribe = subscribeToRoute(() => {
      const fromUrl = parseTimeQuery(readRoute().query)
      if (!fromUrl) return
      if (sameRouteTime(fromUrl, urlWindowSnapshot)) return
      urlWindowSnapshot = fromUrl
      selection =
        fromUrl.type === 'all'
          ? { type: 'all' }
          : { type: 'custom', start: fromUrl.start, end: fromUrl.end }
    })
    return unsubscribe
  })

  const timeContext: TimeContext = {
    get selection() {
      return selection
    },
    get tz() {
      return tz
    },
    setSelection,
    setTz,
  }

  setContext('time', timeContext)
  return timeContext
}

export function getTimeContext(): TimeContext {
  return getContext<TimeContext>('time')
}

export { createTimeContext }
export type { TimeContext, TimeSelection }
