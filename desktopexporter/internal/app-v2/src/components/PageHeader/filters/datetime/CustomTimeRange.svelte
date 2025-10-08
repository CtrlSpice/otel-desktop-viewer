<script lang="ts">
  import * as chrono from 'chrono-node';
  import { getTimeContext } from '@/contexts/time-context.svelte';
  import { formatDateTime } from '@/utils/time';

  // Get time context
  let ctx = getTimeContext();
  if (!ctx) {
    throw new Error(
      'Time context not found. Make sure createTimeContext() is called at the root level.'
    );
  }
  let customStartText = $state('');
  let customEndText = $state('now');
  let customError = $state<string | null>(null);

  type ValidationResult =
    | { isValid: true; start: number; end: number }
    | { isValid: false; error: string };

  type ParseResult =
    | { success: true; timestamp: number }
    | { success: false; error: string };

  // Initialize custom text fields when in custom mode
  $effect(() => {
    if (ctx.selection.type === 'custom') {
      customStartText = formatDateTime(
        ctx.selection.start,
        ctx.timezone,
        'seconds'
      );
      customEndText = formatDateTime(
        ctx.selection.end,
        ctx.timezone,
        'seconds'
      );
      customError = null;
    }
  });

  function validateCustomRange(): ValidationResult {
    // Make sure start is not empty
    if (!customStartText.trim()) {
      return {
        isValid: false,
        error: 'Please enter start time',
      };
    }

    // If end time is empty, set it to now
    if (!customEndText.trim()) {
      customEndText = 'now';
    }

    // Parse start and end times
    let startResult = parseNaturalLanguage(customStartText);
    if (!startResult.success) {
      return {
        isValid: false,
        error: startResult.error,
      };
    }

    let endResult = parseNaturalLanguage(customEndText);
    if (!endResult.success) {
      return {
        isValid: false,
        error: endResult.error,
      };
    }

    let startTime = startResult.timestamp;
    let endTime = endResult.timestamp;

    // Validate start is before end
    if (startTime >= endTime) {
      return {
        isValid: false,
        error: 'Start time must be before end time',
      };
    }

    // Validate not in the future
    if (endTime > Date.now()) {
      return {
        isValid: false,
        error: 'End time cannot be in the future',
      };
    }

    return { isValid: true, start: startTime, end: endTime };
  }

  function applyCustom() {
    customError = null;
    let validation = validateCustomRange();

    if (!validation.isValid) {
      customError = validation.error;
      return; // Don't change the application state
    }

    // Set selection in time context
    ctx.setSelection(validation.start, validation.end, 'custom');
  }

  function parseNaturalLanguage(text: string): ParseResult {
    if (!text.trim()) {
      return { success: false, error: 'Please enter a time' };
    }

    try {
      let parsed = chrono.parseDate(text);
      if (parsed) {
        return { success: true, timestamp: parsed.getTime() };
      } else {
        return {
          success: false,
          error: 'Could not understand this time format',
        };
      }
    } catch (error) {
      return { success: false, error: 'Invalid time format' };
    }
  }

  // Props for different layouts
  let { isNarrow = false } = $props();
</script>

<div class="relative">
  <div
    class="section-header--hide-narrow {ctx.selection.type === 'custom'
      ? 'selection-indicator--active'
      : ''}"
  >
    <svg
      class="w-4 h-4 mr-2"
      fill="none"
      stroke="currentColor"
      viewBox="0 0 24 24"
    >
      <g stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5">
        <path
          d="M16 2v4M8 2v4m13 6c0-3.771 0-5.657-1.172-6.828S16.771 4 13 4h-2C7.229 4 5.343 4 4.172 5.172S3 8.229 3 12v2c0 3.771 0 5.657 1.172 6.828S7.229 22 11 22M3 10h18"
        />
        <path
          d="M18.267 18.701L17 18v-1.733M21 18a4 4 0 1 1-8 0a4 4 0 0 1 8 0"
        />
      </g>
    </svg>
    Custom Time Range
  </div>

  <div class="p-3">
    <!-- From Date/Time -->
    <div class="form-control mx-3">
      <label class="label" for="custom-start{isNarrow ? '-narrow' : ''}">
        <span class="label-text text-sm">Start time:</span>
      </label>
      <input
        id="custom-start{isNarrow ? '-narrow' : ''}"
        type="text"
        placeholder="e.g., 2 hours ago, yesterday, 2024-01-01"
        class="input input-bordered input-sm w-full"
        bind:value={customStartText}
      />
    </div>

    <!-- To Date/Time -->
    <div class="form-control mx-3">
      <label class="label" for="custom-end{isNarrow ? '-narrow' : ''}">
        <span class="label-text text-sm">End time:</span>
      </label>
      <input
        id="custom-end{isNarrow ? '-narrow' : ''}"
        type="text"
        placeholder="e.g., now, 1 hour ago, 2024-01-02"
        class="input input-bordered input-sm w-full"
        bind:value={customEndText}
      />
    </div>

    <!-- Error Display -->
    {#if customError}
      <div class="m-3">
        <div class="alert alert-error alert-sm rounded-md">
          <svg class="w-3 h-3" fill="currentColor" viewBox="0 0 20 20">
            <path
              fill-rule="evenodd"
              d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7 4a1 1 0 11-2 0 1 1 0 012 0zm-1-9a1 1 0 00-1 1v4a1 1 0 102 0V6a1 1 0 00-1-1z"
              clip-rule="evenodd"
            ></path>
          </svg>
          <span class="text-xs">{customError}</span>
        </div>
      </div>
    {/if}

    <!-- Apply Button -->
    <div class="m-3">
      <button class="btn btn-primary btn-sm w-full" onclick={applyCustom}>
        Apply Custom Time Range
      </button>
    </div>
  </div>
</div>
