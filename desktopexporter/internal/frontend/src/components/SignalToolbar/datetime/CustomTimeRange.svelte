<script lang="ts">
  import * as chrono from 'chrono-node'
  import FieldErrorMessage from '@/components/FieldErrorMessage.svelte'
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
  let customFieldIssue = $state<{
    message: string
    invalidFields: ('start' | 'end')[]
  } | null>(null)

  let startInputInvalid = $derived(
    customFieldIssue?.invalidFields.includes('start') ?? false
  )
  let endInputInvalid = $derived(
    customFieldIssue?.invalidFields.includes('end') ?? false
  )

  type ValidationResult =
    | { isValid: true; start: number; end: number }
    | {
        isValid: false
        error: string
        invalidFields: ('start' | 'end')[]
      }

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
      customFieldIssue = null
    } else {
      // Reset fields when selection changes to non-custom type
      customStartText = ''
      customEndText = 'now'
      customFieldIssue = null
    }
  })

  function validateCustomRange(): ValidationResult {
    // Make sure start is not empty
    if (!customStartText.trim()) {
      return {
        isValid: false,
        error: 'Please enter start time',
        invalidFields: ['start'],
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
        invalidFields: ['start'],
      }
    }

    let endResult = parseNaturalLanguage(customEndText)
    if (!endResult.success) {
      return {
        isValid: false,
        error: endResult.error,
        invalidFields: ['end'],
      }
    }

    let startTime = startResult.timestamp
    let endTime = endResult.timestamp

    // Validate start is before end
    if (startTime >= endTime) {
      return {
        isValid: false,
        error: 'Start time must be before end time',
        invalidFields: ['start', 'end'],
      }
    }

    // Validate not in the future
    if (endTime > Date.now()) {
      return {
        isValid: false,
        error: 'End time cannot be in the future',
        invalidFields: ['end'],
      }
    }

    return { isValid: true, start: startTime, end: endTime }
  }

  function applyCustom() {
    customFieldIssue = null
    let validation = validateCustomRange()

    if (!validation.isValid) {
      customFieldIssue = {
        message: validation.error,
        invalidFields: validation.invalidFields,
      }
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
        class="input input-bordered input-sm min-w-0 w-full rounded-lg text-sm"
        class:input-error={startInputInvalid}
        aria-invalid={startInputInvalid}
        aria-describedby={customFieldIssue
          ? 'custom-time-range-error'
          : undefined}
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
          class="input input-bordered input-sm join-item min-w-0 flex-1 rounded-l-lg rounded-r-none text-sm"
          class:input-error={endInputInvalid}
          aria-invalid={endInputInvalid}
          aria-describedby={customFieldIssue
            ? 'custom-time-range-error'
            : undefined}
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

      {#if customFieldIssue}
        <div class="mt-1.5">
          <FieldErrorMessage
            id="custom-time-range-error"
            message={customFieldIssue.message}
          />
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
