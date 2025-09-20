import { setContext, getContext } from 'svelte';

// Base interface with common fields
interface BaseTimeSelection {
  start: number; // Unix timestamp (ms)
  end: number; // Unix timestamp (ms)
  timezone: string;
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
  setSelection: (
    start: number,
    end: number,
    timezone: string,
    type: 'preset' | 'custom' | 'recent',
    presetIndex?: number
  ) => void;
}

// Create the time context with Svelte 5 runes
function createTimeContext(): TimeContext {
  // In time-context.svelte.ts, change the initial selection:
  let selection = $state<TimeSelection>({
    start: 0, // Beginning of time
    end: Date.now(),
    timezone: 'local',
    type: 'preset',
    presetIndex: 8, // "Show all" is index 8 in the PRESETS array
  });

  // Set time selection
  function setSelection(
    start: number,
    end: number,
    timezone: string,
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
          timezone,
          type: 'preset',
          presetIndex: presetIndex,
        };
        break;
      case 'custom':
        selection = { start, end, timezone, type: 'custom' };
        break;
      case 'recent':
        selection = {
          start,
          end,
          timezone,
          type: 'recent',
        };
        break;
    }
    // Save after updating
    localStorage.setItem('time-selection', JSON.stringify(selection));
    console.log('Selection updated to:', selection);
  }

  // Set context for child components
  setContext('time', { selection, setSelection });
  return { selection, setSelection };
}

// Get context in child components
export function getTimeContext(): TimeContext {
  return getContext<TimeContext>('time');
}

// Export the creator function
export { createTimeContext };
export type { TimeContext, TimeSelection };
