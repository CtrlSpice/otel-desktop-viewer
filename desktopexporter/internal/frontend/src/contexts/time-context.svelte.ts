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

function loadTimeSelection(raw: string | null): TimeSelection {
  if (!raw) {
    return {
      start: 0,
      end: Date.now(),
      type: 'preset',
      presetIndex: DEFAULT_PRESET_INDEX_ALL,
    };
  }

  let parsed = JSON.parse(raw) as TimeSelection;

  if (parsed.type === 'preset') {
    return {
      start: parsed.start,
      end: parsed.end,
      type: 'preset',
      presetIndex: parsed.presetIndex,
    };
  }

  return parsed;
}

// Create the time context with Svelte 5 runes
function createTimeContext(): TimeContext {
  let savedSelection = localStorage.getItem('time-selection');
  let savedTimezone = localStorage.getItem('time-timezone') as Timezone | null;

  let selection = $state<TimeSelection>(loadTimeSelection(savedSelection));

  let timezone = $state<Timezone>(savedTimezone || 'local');

  function setSelection(
    start: number,
    end: number,
    type: 'preset' | 'custom' | 'recent',
    presetIndex?: number
  ) {
    switch (type) {
      case 'preset':
        if (typeof presetIndex !== 'number') {
          throw new Error('index is required for preset type');
        }
        selection = {
          start,
          end,
          type: 'preset',
          presetIndex: presetIndex,
        };
        break;
      case 'custom':
        selection = { start, end, type: 'custom' };
        break;
      case 'recent':
        selection = {
          start,
          end,
          type: 'recent',
        };
        break;
    }
    localStorage.setItem('time-selection', JSON.stringify(selection));
    recordRecentTimeRange(start, end, Date.now());
  }

  function setTimezone(newTimezone: Timezone) {
    timezone = newTimezone;
    localStorage.setItem('time-timezone', newTimezone);
  }

  let contextObject = {
    get selection() {
      return selection;
    },
    get timezone() {
      return timezone;
    },
    setSelection,
    setTimezone,
  };

  setContext('time', contextObject);
  return contextObject;
}

export function getTimeContext(): TimeContext {
  return getContext<TimeContext>('time');
}

export { createTimeContext };
export type { TimeContext, TimeSelection };
