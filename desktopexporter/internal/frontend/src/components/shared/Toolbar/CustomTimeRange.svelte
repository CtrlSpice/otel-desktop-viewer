<script lang="ts">
  import { onMount, untrack, type Snippet } from 'svelte'
  import { slide } from 'svelte/transition'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { Calendar03Icon } from '@hugeicons/core-free-icons'
  import {
    CalendarDate,
    parseDate,
    type DateValue,
  } from '@internationalized/date'
  import FieldErrorMessage from '@/components/shared/FieldErrorMessage.svelte'
  import DateCalendar from '@/components/shared/DateCalendar.svelte'
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import {
    formatEditableDateTime,
    formatTimezoneLabel,
    parseDateTimeInTimezone,
    type WallClockDisambiguation,
  } from '@/utils/time'

  let { beforeApply }: { beforeApply?: Snippet } = $props()

  const ctx = getTimeContext()
  type Endpoint = 'start' | 'end'
  type Choice = Exclude<WallClockDisambiguation, 'reject'>
  type Ambiguity = { earlier: number; later: number }
  type EditableParts = { date: string; time: string }
  type InitializedChoice = {
    ambiguity: Ambiguity | null
    choice: Choice | null
  }

  const DATE_PATTERN = /^\d{4}-\d{2}-\d{2}$/
  const TIME_PATTERN = /^(?:[01]\d|2[0-3]):[0-5]\d:[0-5]\d\.\d{3}$/
  const CLOCK_REFRESH_MS = 1000
  const initialNow = Date.now()

  let startDateText = $state('')
  let startTimeText = $state('00:00:00.000')
  let endDateText = $state('')
  let endTimeText = $state('')
  let endTracksNow = $state(true)
  let openCalendar = $state<Endpoint | null>('start')
  let nowTimestamp = $state(initialNow)
  let startChoice = $state<Choice | null>(null)
  let endChoice = $state<Choice | null>(null)
  let startAmbiguity = $state<Ambiguity | null>(null)
  let endAmbiguity = $state<Ambiguity | null>(null)
  let startDirty = false
  let endDirty = false
  let previousSelection = ctx.selection
  let customFieldIssue = $state<{
    message: string
    invalidFields: Endpoint[]
  } | null>(null)

  let startInputInvalid = $derived(
    customFieldIssue?.invalidFields.includes('start') ?? false
  )
  let endInputInvalid = $derived(
    customFieldIssue?.invalidFields.includes('end') ?? false
  )

  type EndpointResult =
    | { isValid: true; timestamp: number }
    | { isValid: false; error: string; ambiguity?: Ambiguity }

  type ValidationResult =
    | { isValid: true; start: number; end: number }
    | {
        isValid: false
        error: string
        invalidFields: Endpoint[]
      }

  let today = $state(calendarDateForTimestamp(initialNow))

  function editableParts(timestamp: number): EditableParts {
    const formatted = formatEditableDateTime(timestamp, ctx.tz)
    return { date: formatted.slice(0, 10), time: formatted.slice(11, 23) }
  }

  function calendarDate(text: string): CalendarDate | undefined {
    if (!DATE_PATTERN.test(text)) return undefined
    try {
      return parseDate(text)
    } catch {
      return undefined
    }
  }

  function calendarDateForTimestamp(timestamp: number): CalendarDate {
    return parseDate(editableParts(timestamp).date)
  }

  function refreshCurrentDay(timestamp: number = Date.now()) {
    const nextToday = calendarDateForTimestamp(timestamp)
    nowTimestamp = timestamp
    if (nextToday.compare(today) !== 0) today = nextToday
    if (endTracksNow && endDateText !== nextToday.toString()) {
      endDateText = nextToday.toString()
      endTimeText = ''
    }
  }

  onMount(() => {
    const timer = setInterval(refreshCurrentDay, CLOCK_REFRESH_MS)
    return () => clearInterval(timer)
  })

  function wallClockText(date: string, time: string): string {
    return `${date} ${time}`
  }

  function ambiguityFor(date: string, time: string): Ambiguity | null {
    const text = wallClockText(date, time)
    const referenceNow = untrack(() => nowTimestamp)
    const earlier = parseDateTimeInTimezone(
      text,
      ctx.tz,
      referenceNow,
      'earlier'
    )
    const later = parseDateTimeInTimezone(text, ctx.tz, referenceNow, 'later')
    if (
      !earlier.success ||
      !later.success ||
      earlier.timestamp === later.timestamp
    ) {
      return null
    }
    return { earlier: earlier.timestamp, later: later.timestamp }
  }

  function initializeChoice(
    date: string,
    time: string,
    timestamp: number
  ): InitializedChoice {
    const ambiguity = ambiguityFor(date, time)
    if (!ambiguity) return { ambiguity: null, choice: null }
    return {
      ambiguity,
      choice: timestamp === ambiguity.later ? 'later' : 'earlier',
    }
  }

  function editStart() {
    startDirty = true
    startChoice = null
    startAmbiguity = null
    customFieldIssue = null
  }

  function editEndField(field: 'date' | 'time', value: string) {
    const now = Date.now()
    const currentDate = editableParts(now).date
    endDirty = true
    endChoice = null
    endAmbiguity = null
    endTracksNow =
      field === 'date'
        ? value === currentDate && endTimeText === ''
        : endDateText === currentDate && value === ''
    refreshCurrentDay(now)
    customFieldIssue = null
  }

  function selectCalendarDate(
    endpoint: Endpoint,
    value: DateValue | undefined
  ) {
    if (!value) return

    if (endpoint === 'start') {
      startDirty = true
      startDateText = value.toString()
      startChoice = null
      startAmbiguity = null
    } else {
      endDirty = true
      endTracksNow = false
      endDateText = value.toString()
      if (!endTimeText) endTimeText = editableParts(nowTimestamp).time
      endChoice = null
      endAmbiguity = null
    }
    customFieldIssue = null
  }

  function resolveEndpoint(
    endpoint: Endpoint,
    date: string,
    time: string,
    choice: Choice | null
  ): EndpointResult {
    if (!calendarDate(date)) {
      return { isValid: false, error: `Enter a valid ${endpoint} date` }
    }
    if (!TIME_PATTERN.test(time)) {
      return {
        isValid: false,
        error: `Enter ${endpoint} time as HH:mm:ss.SSS`,
      }
    }

    const text = wallClockText(date, time)
    const result = parseDateTimeInTimezone(text, ctx.tz, nowTimestamp)
    if (result.success) return { isValid: true, timestamp: result.timestamp }

    const ambiguity = ambiguityFor(date, time)
    if (!ambiguity) return { isValid: false, error: result.error }
    if (!choice) {
      return {
        isValid: false,
        error: `Choose which ${endpoint} time you mean`,
        ambiguity,
      }
    }
    return {
      isValid: true,
      timestamp: choice === 'earlier' ? ambiguity.earlier : ambiguity.later,
    }
  }

  $effect(() => {
    const selection = ctx.selection
    const preserveStart =
      selection === previousSelection && untrack(() => startDirty)
    const preserveEnd =
      selection === previousSelection && untrack(() => endDirty)
    const startDraft = preserveStart
      ? untrack(() => ({ date: startDateText, time: startTimeText }))
      : null
    const endDraft = preserveEnd
      ? untrack(() => ({
          date: endDateText,
          time: endTimeText,
          tracksNow: endTracksNow,
        }))
      : null
    previousSelection = selection
    const now = Date.now()
    const currentDay = calendarDateForTimestamp(now)
    nowTimestamp = now
    today = currentDay

    if (selection.type === 'custom') {
      const start = editableParts(selection.start)
      const end = editableParts(selection.end)
      const initializedStart = initializeChoice(
        start.date,
        start.time,
        selection.start
      )
      const initializedEnd = initializeChoice(end.date, end.time, selection.end)
      startDateText = start.date
      startTimeText = start.time
      endDateText = end.date
      endTimeText = end.time
      startChoice = initializedStart.choice
      endChoice = initializedEnd.choice
      startAmbiguity = initializedStart.ambiguity
      endAmbiguity = initializedEnd.ambiguity
      endTracksNow = false
    } else {
      startDateText = ''
      startTimeText = '00:00:00.000'
      endDateText = currentDay.toString()
      endTimeText = ''
      endTracksNow = true
      startChoice = null
      endChoice = null
      startAmbiguity = null
      endAmbiguity = null
    }
    if (startDraft) {
      startDateText = startDraft.date
      startTimeText = startDraft.time
      startChoice = null
      startAmbiguity = null
    }
    if (endDraft) {
      endDateText = endDraft.tracksNow ? currentDay.toString() : endDraft.date
      endTimeText = endDraft.tracksNow ? '' : endDraft.time
      endTracksNow = endDraft.tracksNow
      endChoice = null
      endAmbiguity = null
    }
    startDirty = preserveStart
    endDirty = preserveEnd
    customFieldIssue = null
  })

  function validateCustomRange(now: number = Date.now()): ValidationResult {
    refreshCurrentDay(now)
    const startResult = resolveEndpoint(
      'start',
      startDateText,
      startTimeText,
      startChoice
    )
    if (!startResult.isValid) {
      startAmbiguity = startResult.ambiguity ?? null
      return {
        isValid: false,
        error: startResult.error,
        invalidFields: ['start'],
      }
    }

    const endResult: EndpointResult = endTracksNow
      ? { isValid: true, timestamp: now }
      : resolveEndpoint('end', endDateText, endTimeText, endChoice)
    if (!endResult.isValid) {
      endAmbiguity = endResult.ambiguity ?? null
      return {
        isValid: false,
        error: endResult.error,
        invalidFields: ['end'],
      }
    }

    if (startResult.timestamp >= endResult.timestamp) {
      return {
        isValid: false,
        error: 'Start time must be before end time',
        invalidFields: ['start', 'end'],
      }
    }

    if (endResult.timestamp > now) {
      return {
        isValid: false,
        error: 'End time cannot be in the future',
        invalidFields: ['end'],
      }
    }

    return {
      isValid: true,
      start: startResult.timestamp,
      end: endResult.timestamp,
    }
  }

  function applyCustom() {
    customFieldIssue = null
    const validation = validateCustomRange()

    if (!validation.isValid) {
      customFieldIssue = {
        message: validation.error,
        invalidFields: validation.invalidFields,
      }
      return
    }

    startDirty = false
    endDirty = false
    ctx.setSelection({
      type: 'custom',
      start: validation.start,
      end: validation.end,
    })
  }

  function ambiguityLabel(timestamp: number, position: Choice): string {
    const abbreviation = formatTimezoneLabel(ctx.tz, new Date(timestamp))
    const offset = formatEditableDateTime(timestamp, ctx.tz).slice(23)
    return `${position === 'earlier' ? 'Earlier' : 'Later'} · ${abbreviation} · ${offset}`
  }
</script>

<form
  class="min-w-0 w-full"
  onsubmit={e => {
    e.preventDefault()
    applyCustom()
  }}
>
  <fieldset class="custom-range-layout">
    <legend class="sr-only">Custom Time Range</legend>

    {#snippet endpointCalendar(endpoint: Endpoint)}
      <DateCalendar
        id={`custom-${endpoint}-calendar`}
        label={`${endpoint === 'start' ? 'Start' : 'End'} date`}
        value={calendarDate(endpoint === 'start' ? startDateText : endDateText)}
        onValueChange={value => selectCalendarDate(endpoint, value)}
        placeholder={calendarDate(
          endpoint === 'start' ? startDateText : endDateText
        ) ?? today}
        maxValue={today}
      />
    {/snippet}

    <div class="range-editor-row">
      <label for="custom-start-date" class="endpoint-label"> Start: </label>
      <div class="endpoint-fields">
        <input
          id="custom-start-date"
          type="text"
          aria-label="Start"
          placeholder="YYYY-MM-DD"
          class="endpoint-input endpoint-date"
          class:input-error={startInputInvalid}
          aria-invalid={startInputInvalid}
          aria-describedby={startInputInvalid
            ? 'custom-time-range-error'
            : undefined}
          oninput={editStart}
          bind:value={startDateText}
        />
        <span aria-hidden="true">,</span>
        <input
          id="custom-start-time"
          type="text"
          aria-label="Start time"
          placeholder="HH:mm:ss.SSS"
          class="endpoint-input"
          class:input-error={startInputInvalid}
          aria-invalid={startInputInvalid}
          aria-describedby={startInputInvalid
            ? 'custom-time-range-error'
            : undefined}
          oninput={editStart}
          bind:value={startTimeText}
        />
      </div>
      <button
        type="button"
        class="endpoint-calendar-button btn btn-circle btn-ghost btn-xs"
        aria-label="Choose start date"
        aria-expanded={openCalendar === 'start'}
        aria-controls="custom-start-calendar"
        onclick={() =>
          (openCalendar = openCalendar === 'start' ? null : 'start')}
      >
        <HugeiconsIcon icon={Calendar03Icon} size={14} aria-hidden="true" />
      </button>
    </div>

    {#if openCalendar === 'start'}
      <div class="min-w-0" transition:slide={{ duration: 120 }}>
        {@render endpointCalendar('start')}
      </div>
    {/if}

    {#if startAmbiguity}
      <div
        class="ambiguity-choices"
        role="group"
        aria-label="Start time occurrence"
      >
        <button
          type="button"
          class="ambiguity-choice item-state"
          aria-pressed={startChoice === 'earlier'}
          onclick={() => {
            startDirty = true
            startChoice = 'earlier'
            customFieldIssue = null
          }}
        >
          {ambiguityLabel(startAmbiguity.earlier, 'earlier')}
        </button>
        <button
          type="button"
          class="ambiguity-choice item-state"
          aria-pressed={startChoice === 'later'}
          onclick={() => {
            startDirty = true
            startChoice = 'later'
            customFieldIssue = null
          }}
        >
          {ambiguityLabel(startAmbiguity.later, 'later')}
        </button>
      </div>
    {/if}

    <div class="range-editor-row">
      <label for="custom-end-date" class="endpoint-label"> End: </label>
      <div class="endpoint-fields">
        <input
          id="custom-end-date"
          type="text"
          aria-label="End"
          placeholder="YYYY-MM-DD"
          class="endpoint-input endpoint-date"
          class:input-error={endInputInvalid}
          aria-invalid={endInputInvalid}
          aria-describedby={endInputInvalid
            ? 'custom-time-range-error'
            : undefined}
          oninput={event => editEndField('date', event.currentTarget.value)}
          bind:value={endDateText}
        />
        <span aria-hidden="true">,</span>
        <input
          id="custom-end-time"
          type="text"
          aria-label="End time"
          placeholder="right now"
          class="endpoint-input"
          class:input-error={endInputInvalid}
          aria-invalid={endInputInvalid}
          aria-describedby={endInputInvalid
            ? 'custom-time-range-error'
            : undefined}
          oninput={event => editEndField('time', event.currentTarget.value)}
          bind:value={endTimeText}
        />
      </div>
      <button
        type="button"
        class="endpoint-calendar-button btn btn-circle btn-ghost btn-xs"
        aria-label="Choose end date"
        aria-expanded={openCalendar === 'end'}
        aria-controls="custom-end-calendar"
        onclick={() => (openCalendar = openCalendar === 'end' ? null : 'end')}
      >
        <HugeiconsIcon icon={Calendar03Icon} size={14} aria-hidden="true" />
      </button>
    </div>

    {#if openCalendar === 'end'}
      <div class="min-w-0" transition:slide={{ duration: 120 }}>
        {@render endpointCalendar('end')}
      </div>
    {/if}

    {#if endAmbiguity}
      <div
        class="ambiguity-choices"
        role="group"
        aria-label="End time occurrence"
      >
        <button
          type="button"
          class="ambiguity-choice item-state"
          aria-pressed={endChoice === 'earlier'}
          onclick={() => {
            endDirty = true
            endChoice = 'earlier'
            customFieldIssue = null
          }}
        >
          {ambiguityLabel(endAmbiguity.earlier, 'earlier')}
        </button>
        <button
          type="button"
          class="ambiguity-choice item-state"
          aria-pressed={endChoice === 'later'}
          onclick={() => {
            endDirty = true
            endChoice = 'later'
            customFieldIssue = null
          }}
        >
          {ambiguityLabel(endAmbiguity.later, 'later')}
        </button>
      </div>
    {/if}

    {#if beforeApply}{@render beforeApply()}{/if}

    <button
      type="submit"
      class="btn btn-sm btn-ghost mt-2 w-full rounded-full text-sm text-primary"
      aria-label="Apply range"
    >
      Apply
    </button>
    {#if customFieldIssue}
      <FieldErrorMessage
        id="custom-time-range-error"
        message={customFieldIssue.message}
      />
    {/if}
  </fieldset>
</form>

<style lang="postcss">
  @reference "../../../app.css";

  .custom-range-layout {
    @apply m-0 flex min-w-0 w-full flex-col border-0 p-0;
  }

  .range-editor-row {
    @apply grid grid-cols-[3.5rem_minmax(0,1fr)_auto] items-center gap-x-3;
  }

  .range-editor-row:has(input:focus) {
    box-shadow: inset 0 0 0 var(--focus-ring-width) var(--focus-ring-color);
  }

  .range-editor-row:has(input.input-error:focus) {
    box-shadow: inset 0 0 0 var(--focus-ring-width) var(--color-error);
  }

  .endpoint-label {
    @apply cursor-pointer pl-3 text-sm font-normal;
    color: var(--color-subtle);
  }

  .endpoint-input {
    @apply min-w-0 max-w-full rounded-none border-0 bg-transparent p-0 text-left font-mono text-sm tracking-tight tabular-nums shadow-none outline-none;
    field-sizing: content;
    height: calc(var(--table-row-h) + 0.5rem);
    line-height: calc(var(--table-row-h) + 0.5rem);
  }

  .endpoint-input::placeholder {
    color: var(--color-subtle);
    opacity: 1;
  }

  .endpoint-fields {
    @apply flex min-w-0 items-center justify-end font-mono text-sm;
  }

  .endpoint-fields > span {
    @apply pr-2 text-base-content/50;
  }

  .endpoint-date {
    @apply w-[10ch] shrink-0;
  }

  .endpoint-calendar-button {
    @apply mr-1 h-7 min-h-0 w-7 border-transparent p-0 text-base-content/50 shadow-none;
    @apply hover:bg-base-300/40 hover:text-base-content;
  }

  .endpoint-calendar-button[aria-expanded='true'] :global(svg) {
    color: var(--color-primary);
  }

  .ambiguity-choices {
    @apply grid grid-cols-2 gap-1 px-2 py-1.5;
  }

  .ambiguity-choice {
    @apply min-w-0 truncate rounded-full border border-base-300 px-2 py-1 font-mono text-[0.625rem] text-base-content;
  }
</style>
