<script lang="ts">
  import * as chrono from 'chrono-node'
  import FieldErrorMessage from '@/components/shared/FieldErrorMessage.svelte'
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import { formatDateTimeMs } from '@/utils/time'

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
      customStartText = formatDateTimeMs(
        ctx.selection.start,
        ctx.timezone
      ).dateTime
      customEndText = formatDateTimeMs(ctx.selection.end, ctx.timezone).dateTime
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

<form
  class="min-w-0 w-full"
  onsubmit={e => {
    e.preventDefault()
    applyCustom()
  }}
>
  <fieldset class="fieldset min-w-0 w-full px-0 py-0">
    <legend class="fieldset-legend sr-only">Custom Time Range</legend>

    <div class="flex min-w-0 w-full flex-col gap-2">
      <div class="typed-field-group join w-full">
        <label for="custom-start" class="typed-field-label join-item">
          Start:<span class="sr-only"> time</span>
        </label>
        <input
          id="custom-start"
          type="text"
          placeholder="yesterday, 2024-01-01, 2 hours ago"
          class="typed-field input input-sm join-item"
          class:input-error={startInputInvalid}
          aria-invalid={startInputInvalid}
          aria-describedby={customFieldIssue
            ? 'custom-time-range-error'
            : undefined}
          bind:value={customStartText}
        />
      </div>

      <div class="typed-field-group join w-full">
        <label for="custom-end" class="typed-field-label join-item">
          End:<span class="sr-only"> time</span>
        </label>
        <input
          id="custom-end"
          type="text"
          placeholder="e.g., now, 1 hour ago, 2024-01-02"
          class="typed-field input input-sm join-item"
          class:input-error={endInputInvalid}
          aria-invalid={endInputInvalid}
          aria-describedby={customFieldIssue
            ? 'custom-time-range-error'
            : undefined}
          bind:value={customEndText}
        />
        <button
          type="submit"
          class="typed-field typed-field--action typed-field--apply btn btn-sm join-item shrink-0 gap-1"
          title="Apply"
        >
          <svg class="h-3.5 w-3.5" viewBox="0 0 24 24" aria-hidden="true">
            <path d="M5 14l3.5 3.5L19 6.5" />
          </svg>
          Apply
        </button>
      </div>

      {#if customFieldIssue}
        <div>
          <FieldErrorMessage
            id="custom-time-range-error"
            message={customFieldIssue.message}
          />
        </div>
      {/if}
    </div>
  </fieldset>
</form>
