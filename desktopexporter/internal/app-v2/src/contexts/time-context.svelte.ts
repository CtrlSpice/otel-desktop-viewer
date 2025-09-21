import { setContext, getContext } from 'svelte';
import type { Timezone } from '../utils/time';

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
  // Time selection state
  let selection = $state<TimeSelection>({
    start: 0, // Beginning of time
    end: Date.now(),
    type: 'preset',
    presetIndex: 8, // "Show all" is index 8 in the PRESETS array
  });

  // Timezone state
  let timezone = $state<Timezone>('local');

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
    // Save after updating
    localStorage.setItem('time-selection', JSON.stringify(selection));
    console.log('Selection updated to:', selection);
  }

  // Set timezone
  function setTimezone(newTimezone: Timezone) {
    timezone = newTimezone;
    // Save timezone separately
    localStorage.setItem('time-timezone', newTimezone);
  }

  // Set context for child components
  setContext('time', { selection, timezone, setSelection, setTimezone });
  return { selection, timezone, setSelection, setTimezone };
}

// Get context in child components
export function getTimeContext(): TimeContext {
  return getContext<TimeContext>('time');
}

// Export the creator function
export { createTimeContext };
export type { TimeContext, TimeSelection };
