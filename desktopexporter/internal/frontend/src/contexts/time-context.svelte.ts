import { setContext, getContext } from 'svelte'
import { type Timezone, recordRecentTimeRange } from '@/utils/time'
import {
  navigateCurrentRoute,
  readRoute,
  subscribeToRoute,
  withQueryPatch,
} from '@/route'

// Base interface with common fields
interface BaseTimeSelection {
  start: number // Unix timestamp (ms)
  end: number // Unix timestamp (ms)
}

// Type-specific extensions using discriminated unions
type TimeSelection = BaseTimeSelection &
  (
    | { type: 'preset'; presetIndex: number }
    | { type: 'custom' }
    | { type: 'recent' }
  )

/**
 * Unix ms range for search/export APIs.
 * Presets use stored start/end only as a duration, anchored so the window ends at `nowMs`.
 * Custom and recent use the stored bounds as-is.
 */
export function selectionToQueryRangeMs(
  selection: TimeSelection,
  nowMs: number
): { start: number; end: number } {
  if (selection.type === 'preset') {
    const duration = selection.end - selection.start
    const end = nowMs
    const start = end - duration
    return { start, end }
  }
  return { start: selection.start, end: selection.end }
}

interface TimeContext {
  selection: TimeSelection
  tz: Timezone
  setSelection: (
    start: number,
    end: number,
    type: 'preset' | 'custom' | 'recent',
    presetIndex?: number
  ) => void
  setTz: (tz: Timezone) => void
}

/** Default preset row index for `PresetTimeRanges` PRESETS (0 = All). */
const DEFAULT_PRESET_INDEX_ALL = 0

/** Build selection state; `preset` uses `presetIndex ?? DEFAULT_PRESET_INDEX_ALL` when omitted. */
function timeSelectionFromArgs(
  start: number,
  end: number,
  type: 'preset' | 'custom' | 'recent',
  presetIndex?: number
): TimeSelection {
  return type === 'preset'
    ? {
        start,
        end,
        type: 'preset',
        presetIndex: presetIndex ?? DEFAULT_PRESET_INDEX_ALL,
      }
    : { start, end, type }
}

/** Restore from localStorage string, or default preset (0 → now) via `timeSelectionFromArgs`. */
function loadTimeSelection(raw: string | null, nowMs: number): TimeSelection {
  if (!raw) {
    return timeSelectionFromArgs(0, nowMs, 'preset')
  }

  let parsed: TimeSelection
  try {
    parsed = JSON.parse(raw) as TimeSelection
  } catch {
    return timeSelectionFromArgs(0, nowMs, 'preset')
  }

  return timeSelectionFromArgs(
    parsed.start,
    parsed.end,
    parsed.type,
    'presetIndex' in parsed ? parsed.presetIndex : undefined
  )
}

function parseTimezone(value: string | null): Timezone | null {
  return value === 'local' || value === 'UTC' ? value : null
}

/** Parse `start`/`end` from the route query (Unix ms). */
function parseTimeQuery(
  query: Record<string, string>
): { start: number; end: number } | null {
  const start = Number(query.start)
  const end = Number(query.end)
  if (
    !query.start ||
    !query.end ||
    !Number.isFinite(start) ||
    !Number.isFinite(end)
  ) {
    return null
  }
  return { start, end }
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
  const savedTimezone = localStorage.getItem('time-timezone') as Timezone | null

  const now = Date.now()
  const urlTime = parseTimeQuery(readRoute().query)

  let selection = $state<TimeSelection>(
    urlTime
      ? timeSelectionFromArgs(urlTime.start, urlTime.end, 'custom')
      : loadTimeSelection(savedSelection, now)
  )
  let tz = $state<Timezone>(parseTimezone(savedTimezone) ?? 'local')

  // Remember the window we last pushed to the URL so the router subscription can
  // tell our own (frozen) writes apart from external changes (back/forward,
  // shared links). Presets stay live in memory while the URL holds a frozen
  // absolute snapshot, so we must not treat that mismatch as an external edit.
  let lastWrittenWindow: { start: number; end: number } | null = urlTime
    ? { start: urlTime.start, end: urlTime.end }
    : null

  function syncUrl() {
    const range = selectionToQueryRangeMs(selection, Date.now())
    lastWrittenWindow = range
    navigateCurrentRoute(
      withQueryPatch(readRoute().query, {
        start: String(range.start),
        end: String(range.end),
      }),
      { replace: true }
    )
  }

  function setSelection(
    start: number,
    end: number,
    type: 'preset' | 'custom' | 'recent',
    presetIndex?: number
  ) {
    const now = Date.now()
    selection = timeSelectionFromArgs(start, end, type, presetIndex)
    localStorage.setItem('time-selection', JSON.stringify(selection))
    recordRecentTimeRange(start, end, now)
    syncUrl()
  }

  function setTz(newTz: Timezone) {
    tz = newTz
    localStorage.setItem('time-timezone', newTz)
  }

  // Adopt the window from the URL on external changes only. An external change
  // is one whose absolute bounds differ from what we last wrote, so item
  // navigation (which leaves the time query untouched) and our own writes are
  // both ignored — no feedback loop, no clobbering live presets.
  $effect(() => {
    const unsubscribe = subscribeToRoute(() => {
      const fromUrl = parseTimeQuery(readRoute().query)
      if (!fromUrl) return
      if (
        lastWrittenWindow &&
        fromUrl.start === lastWrittenWindow.start &&
        fromUrl.end === lastWrittenWindow.end
      ) {
        return
      }
      lastWrittenWindow = { start: fromUrl.start, end: fromUrl.end }
      selection = timeSelectionFromArgs(fromUrl.start, fromUrl.end, 'custom')
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
