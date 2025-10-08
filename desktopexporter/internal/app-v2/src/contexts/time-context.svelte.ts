import { setContext, getContext } from 'svelte';
import type { Timezone } from '@/utils/time';

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

// Create the time context with Svelte 5 runes
function createTimeContext(): TimeContext {
  let savedSelection = localStorage.getItem('time-selection');
  let savedTimezone = localStorage.getItem('time-timezone') as Timezone | null;

  // Time selection state
  let selection = $state<TimeSelection>(
    savedSelection
      ? JSON.parse(savedSelection)
      : {
          start: 0,
          end: Date.now(),
          type: 'preset',
          presetIndex: 9, // "Show all" is index 9 in the PRESETS array
        }
  );

  // Timezone state
  let timezone = $state<Timezone>(savedTimezone || 'local');

  // Set time selection
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
  }

  // Set timezone
  function setTimezone(newTimezone: Timezone) {
    timezone = newTimezone;
    localStorage.setItem('time-timezone', newTimezone);
  }

  // Create reactive context object
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

  // Set context for child components
  setContext('time', contextObject);
  return contextObject;
}

// Get context in child components
export function getTimeContext(): TimeContext {
  return getContext<TimeContext>('time');
}

// Export the creator function
export { createTimeContext };
export type { TimeContext, TimeSelection };
