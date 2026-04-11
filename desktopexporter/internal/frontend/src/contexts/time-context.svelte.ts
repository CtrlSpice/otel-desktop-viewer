import { setContext, getContext } from 'svelte';
import type { Timezone } from '@/utils/time';
import { recordRecentTimeRange } from '@/utils/recent-time-ranges';

// Base interface with common fields
interface BaseTimeSelection {
  start: number; // Unix timestamp (ms)
  end: number; // Unix timestamp (ms)
}

// Type-specific extensions using discriminated unions
type TimeSelection = BaseTimeSelection &
  (
    | { type: 'preset'; presetIndex: number }
    | { type: 'custom' }
    | { type: 'recent' }
  );

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
  selection: TimeSelection;
  timezone: Timezone;
  setSelection: (
    start: number,
    end: number,
    type: 'preset' | 'custom' | 'recent',
    presetIndex?: number
  ) => void;
  setTimezone: (timezone: Timezone) => void;
}

/** Default preset row index for `PresetTimeRanges` PRESETS (0 = All). */
const DEFAULT_PRESET_INDEX_ALL = 0;

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

/** Read/write localStorage; hold reactive selection + timezone. */
function createTimeContext(): TimeContext {
  const savedSelection = localStorage.getItem('time-selection')
  const savedTimezone = localStorage.getItem('time-timezone') as Timezone | null

  const now = Date.now()
  let selection = $state<TimeSelection>(loadTimeSelection(savedSelection, now))
  let timezone = $state<Timezone>(savedTimezone ?? 'local')

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
  }

  function setTimezone(newTimezone: Timezone) {
    timezone = newTimezone
    localStorage.setItem('time-timezone', newTimezone)
  }

  const timeContext: TimeContext = {
    get selection() {
      return selection
    },
    get timezone() {
      return timezone
    },
    setSelection,
    setTimezone,
  }

  setContext('time', timeContext)
  return timeContext
}

export function getTimeContext(): TimeContext {
  return getContext<TimeContext>('time');
}

export { createTimeContext };
export type { TimeContext, TimeSelection };
