<script lang="ts">
  import * as chrono from 'chrono-node'
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import { formatDateTime } from '@/utils/time'

  // Get time context
  let ctx = getTimeContext()
  if (!ctx) {
    throw new Error(
      'Time context not found. Make sure createTimeContext() is called at the root level.'
    )
  }
  let customStartText = $state('')
  let customEndText = $state('now')
  let customError = $state<string | null>(null)

  type ValidationResult =
    | { isValid: true; start: number; end: number }
    | { isValid: false; error: string }

  type ParseResult =
    | { success: true; timestamp: number }
    | { success: false; error: string }

  // Initialize custom text fields when in custom mode
  $effect(() => {
    if (ctx.selection.type === 'custom') {
      customStartText = formatDateTime(
        ctx.selection.start,
        ctx.timezone,
        'seconds'
      )
      customEndText = formatDateTime(ctx.selection.end, ctx.timezone, 'seconds')
      customError = null
    } else {
      // Reset fields when selection changes to non-custom type
      customStartText = ''
      customEndText = 'now'
      customError = null
    }
  })

  function validateCustomRange(): ValidationResult {
    // Make sure start is not empty
    if (!customStartText.trim()) {
      return {
        isValid: false,
        error: 'Please enter start time',
      }
    }

    // If end time is empty, set it to now
    if (!customEndText.trim()) {
      customEndText = 'now'
    }

    // Parse start and end times
    let startResult = parseNaturalLanguage(customStartText)
    if (!startResult.success) {
      return {
        isValid: false,
        error: startResult.error,
      }
    }

    let endResult = parseNaturalLanguage(customEndText)
    if (!endResult.success) {
      return {
        isValid: false,
        error: endResult.error,
      }
    }

    let startTime = startResult.timestamp
    let endTime = endResult.timestamp

    // Validate start is before end
    if (startTime >= endTime) {
      return {
        isValid: false,
        error: 'Start time must be before end time',
      }
    }

    // Validate not in the future
    if (endTime > Date.now()) {
      return {
        isValid: false,
        error: 'End time cannot be in the future',
      }
    }

    return { isValid: true, start: startTime, end: endTime }
  }

  function applyCustom() {
    customError = null
    let validation = validateCustomRange()

    if (!validation.isValid) {
      customError = validation.error
      return // Don't change the application state
    }

    // Set selection in time context
    ctx.setSelection(validation.start, validation.end, 'custom')
  }

  function parseNaturalLanguage(text: string): ParseResult {
    if (!text.trim()) {
      return { success: false, error: 'Please enter a time' }
    }

    try {
      let parsed = chrono.parseDate(text)
      if (parsed) {
        return { success: true, timestamp: parsed.getTime() }
      } else {
        return {
          success: false,
          error: 'Could not understand this time format',
        }
      }
    } catch (error) {
      return { success: false, error: 'Invalid time format' }
    }
  }
</script>

<div class="min-w-0 w-full">
  <form
    onsubmit={e => {
      e.preventDefault()
      applyCustom()
    }}
  >
    <fieldset class="fieldset min-w-0 w-full p-3">
      <legend class="fieldset-legend sr-only">Custom Time Range</legend>

      <label class="label" for="custom-start">
        <span class="table-header-typography">Start Time</span>
      </label>
      <input
        id="custom-start"
        type="text"
        placeholder="yesterday, 2024-01-01, 2 hours ago"
        class="input input-bordered input-sm min-w-0 w-full rounded-lg font-mono text-sm"
        bind:value={customStartText}
      />

      <label class="label" for="custom-end">
        <span class="table-header-typography">End Time</span>
      </label>
      <div class="custom-time-join join w-full">
        <input
          id="custom-end"
          type="text"
          placeholder="e.g., now, 1 hour ago, 2024-01-02"
          class="input input-bordered input-sm join-item min-w-0 flex-1 rounded-l-lg rounded-r-none font-mono text-sm"
          bind:value={customEndText}
        />
        <button
          type="submit"
          class="btn btn-soft btn-primary btn-sm join-item shrink-0 gap-1 rounded-l-none rounded-r-lg font-semibold"
          title="Apply"
        >
          <svg class="h-3.5 w-3.5" viewBox="0 0 24 24" aria-hidden="true">
            <path d="M5 14l3.5 3.5L19 6.5" />
          </svg>
          Apply
        </button>
      </div>

      {#if customError}
        <div
          class="flex items-start gap-1.5 bg-transparent text-error"
          role="alert"
        >
          <svg
            class="mt-0.5 h-3 w-3 shrink-0"
            fill="currentColor"
            viewBox="0 0 20 20"
            aria-hidden="true"
          >
            <path
              fill-rule="evenodd"
              d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7 4a1 1 0 11-2 0 1 1 0 012 0zm-1-9a1 1 0 00-1 1v4a1 1 0 102 0V6a1 1 0 00-1-1z"
              clip-rule="evenodd"
            ></path>
          </svg>
          <span class="text-sm leading-snug">{customError}</span>
        </div>
      {/if}
    </fieldset>
  </form>
</div>

<style lang="postcss">
  @reference "../../../app.css";

  /* Seam: no double border between input and button (same width as input border). */
  .custom-time-join :global(input.join-item) {
    border-right-width: 0;
  }

  /* Soft-primary border to match btn-soft btn-primary (see toolbar-filter-trigger--compact). */
  .custom-time-join :global(input.join-item + button.join-item) {
    @apply border border-solid border-primary/25;
  }
</style>
