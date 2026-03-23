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
    } else {
      // Reset fields when selection changes to non-custom type
      customStartText = '';
      customEndText = 'now';
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

</script>

<div class="relative pl-2">
  <div class="section-header mb-2 flex items-center text-sm font-semibold text-base-content">
    {#if ctx.selection.type === 'custom'}
      <svg class="w-4 h-4 mr-2 text-primary" viewBox="0 0 24 24">
        <path d="m5 14l3.5 3.5L19 6.5" />
      </svg>
    {:else}
      <svg
        class="w-4 h-4 mr-2"
        viewBox="0 0 24 24"
      >
        <g>
          <path
            d="M16 2v4M8 2v4m13 6c0-3.771 0-5.657-1.172-6.828S16.771 4 13 4h-2C7.229 4 5.343 4 4.172 5.172S3 8.229 3 12v2c0 3.771 0 5.657 1.172 6.828S7.229 22 11 22M3 10h18"
          />
          <path
            d="M18.267 18.701L17 18v-1.733M21 18a4 4 0 1 1-8 0a4 4 0 0 1 8 0"
          />
        </g>
      </svg>
    {/if}
    Custom Time Range
  </div>

  <div class="pb-3 pr-2">
    <!-- From Date/Time -->
    <div class="flex items-center gap-2 mb-3">
      <label class="label py-0 w-18" for="custom-start">
        <span class="label-text text-sm whitespace-nowrap">Start time:</span>
      </label>
      <input
        id="custom-start"
        type="text"
        placeholder="e.g., 2 hours ago, yesterday, 2024-01-01"
        class="input input-bordered input-sm flex-1"
        bind:value={customStartText}
      />
    </div>

    <!-- To Date/Time -->
    <div class="flex items-center gap-2 mb-3">
      <label class="label py-0 w-18" for="custom-end">
        <span class="label-text text-sm whitespace-nowrap">End time:</span>
      </label>
      <input
        id="custom-end"
        type="text"
        placeholder="e.g., now, 1 hour ago, 2024-01-02"
        class="input input-bordered input-sm flex-1"
        bind:value={customEndText}
      />
    </div>

    <!-- Error Display -->
    {#if customError}
      <div class="mb-3">
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
    <div class="mt-3">
      <button class="btn btn-primary btn-sm w-full" onclick={applyCustom}>
        Apply
      </button>
    </div>
  </div>
</div>
