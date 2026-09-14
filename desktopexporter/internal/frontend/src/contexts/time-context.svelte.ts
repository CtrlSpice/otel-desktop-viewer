import { createContext } from 'svelte'
import {
  type Timezone,
  isDateTimestamp,
  normalizeTimezone,
  recordRecentTimeRange,
} from '@/utils/time'
import {
  navigateCurrentRoute,
  readRoute,
  subscribeToRoute,
  withQueryPatch,
} from '@/route'

export const TIME_RANGE_PRESETS = [
  { label: 'All', durationMs: undefined },
  { label: '5m', durationMs: 300_000 },
  { label: '15m', durationMs: 900_000 },
  { label: '30m', durationMs: 1_800_000 },
  { label: '1h', durationMs: 3_600_000 },
  { label: '6h', durationMs: 21_600_000 },
  { label: '24h', durationMs: 86_400_000 },
  { label: '7d', durationMs: 604_800_000 },
] as const

const [getTimeContext, setTimeContext] = createContext<TimeContext>()

type PresetDurationMs = Exclude<
  (typeof TIME_RANGE_PRESETS)[number]['durationMs'],
  undefined
>

type TimeSelection =
  | { type: 'all' }
  | { type: 'preset'; durationMs: PresetDurationMs }
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

type TimeSelectionFields = {
  type?: unknown
  durationMs?: unknown
  start?: unknown
  end?: unknown
}

function isTimeSelectionFields(value: unknown): value is TimeSelectionFields {
  return value !== null && typeof value === 'object'
}

function hasOnlyKeys(
  value: TimeSelectionFields,
  keys: readonly string[]
): boolean {
  const actual = Object.keys(value)
  return actual.length === keys.length && keys.every(key => key in value)
}

function isBoundedSelection(
  value: TimeSelectionFields
): value is Extract<TimeSelection, { type: 'custom' | 'recent' }> {
  return (
    (value.type === 'custom' || value.type === 'recent') &&
    hasOnlyKeys(value, ['type', 'start', 'end']) &&
    isDateTimestamp(value.start) &&
    isDateTimestamp(value.end) &&
    value.start < value.end
  )
}

function isTimeSelection(value: unknown): value is TimeSelection {
  if (!isTimeSelectionFields(value)) return false
  if (value.type === 'all') return hasOnlyKeys(value, ['type'])
  if (isBoundedSelection(value)) return true
  if (value.type !== 'preset' || !hasOnlyKeys(value, ['type', 'durationMs'])) {
    return false
  }
  return (
    typeof value.durationMs === 'number' &&
    TIME_RANGE_PRESETS.some(preset => preset.durationMs === value.durationMs)
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
    !isDateTimestamp(start) ||
    !isDateTimestamp(end) ||
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

  setTimeContext(timeContext)
  return timeContext
}

export { createTimeContext, getTimeContext }
export type { TimeContext, TimeSelection }
