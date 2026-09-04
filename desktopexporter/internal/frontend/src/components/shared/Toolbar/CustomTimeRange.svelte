<script lang="ts">
  import { onMount, untrack } from 'svelte'
  import { slide } from 'svelte/transition'
  import { Calendar } from 'bits-ui'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import ArrowLeft01Icon from '@hugeicons/core-free-icons/ArrowLeft01Icon'
  import ArrowRight01Icon from '@hugeicons/core-free-icons/ArrowRight01Icon'
  import Calendar03Icon from '@hugeicons/core-free-icons/Calendar03Icon'
  import {
    CalendarDate,
    parseDate,
    type DateValue,
  } from '@internationalized/date'
  import FieldErrorMessage from '@/components/shared/FieldErrorMessage.svelte'
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import {
    formatEditableDateTime,
    formatTimezoneLabel,
    parseDateTimeInTimezone,
    type WallClockDisambiguation,
  } from '@/utils/time'

  // Get time context
  let ctx = getTimeContext()
  if (!ctx) {
    throw new Error(
      'Time context not found. Make sure createTimeContext() is called at the root level.'
    )
  }
  type Endpoint = 'start' | 'end'
  type Choice = Exclude<WallClockDisambiguation, 'reject'>
  type Ambiguity = { earlier: number; later: number }

  const DATE_PATTERN = /^\d{4}-\d{2}-\d{2}$/
  const TIME_PATTERN = /^(?:[01]\d|2[0-3]):[0-5]\d:[0-5]\d\.\d{3}$/
  const CLOCK_REFRESH_MS = 1000
  const initialNow = Date.now()

  let startDateText = $state('')
  let startTimeText = $state('00:00:00.000')
  let endDateText = $state('')
  let endTimeText = $state('')
  let endTracksNow = $state(true)
  let openCalendar = $state<Endpoint | null>(null)
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

  function editableParts(timestamp: number): { date: string; time: string } {
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
  ): { ambiguity: Ambiguity | null; choice: Choice | null } {
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
    openCalendar = null
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
  <fieldset class="fieldset min-w-0 w-full px-0 py-0">
    <legend class="fieldset-legend sr-only">Custom Time Range</legend>

    {#snippet endpointCalendar(endpoint: Endpoint)}
      <Calendar.Root
        id={`custom-${endpoint}-calendar`}
        type="single"
        class="custom-calendar"
        value={calendarDate(endpoint === 'start' ? startDateText : endDateText)}
        onValueChange={value => selectCalendarDate(endpoint, value)}
        placeholder={calendarDate(
          endpoint === 'start' ? startDateText : endDateText
        ) ?? today}
        maxValue={today}
        preventDeselect={true}
        initialFocus={true}
        calendarLabel={`${endpoint === 'start' ? 'Start' : 'End'} date`}
        weekdayFormat="short"
        fixedWeeks={true}
      >
        {#snippet children({ months, weekdays })}
          <Calendar.Header class="calendar-header">
            <Calendar.PrevButton class="calendar-nav">
              <HugeiconsIcon
                icon={ArrowLeft01Icon}
                size={14}
                aria-hidden="true"
              />
            </Calendar.PrevButton>
            <Calendar.Heading class="calendar-heading" />
            <Calendar.NextButton class="calendar-nav">
              <HugeiconsIcon
                icon={ArrowRight01Icon}
                size={14}
                aria-hidden="true"
              />
            </Calendar.NextButton>
          </Calendar.Header>

          {#each months as month (month.value.toString())}
            <Calendar.Grid class="calendar-grid">
              <Calendar.GridHead>
                <Calendar.GridRow>
                  {#each weekdays as weekday (weekday)}
                    <Calendar.HeadCell class="calendar-weekday">
                      {weekday.slice(0, 2)}
                    </Calendar.HeadCell>
                  {/each}
                </Calendar.GridRow>
              </Calendar.GridHead>
              <Calendar.GridBody>
                {#each month.weeks as weekDates, week (week)}
                  <Calendar.GridRow>
                    {#each weekDates as date (date.toString())}
                      <Calendar.Cell
                        {date}
                        month={month.value}
                        class="calendar-cell"
                      >
                        <Calendar.Day class="calendar-day">
                          <span class="calendar-today-dot" aria-hidden="true"
                          ></span>
                          {date.day}
                        </Calendar.Day>
                      </Calendar.Cell>
                    {/each}
                  </Calendar.GridRow>
                {/each}
              </Calendar.GridBody>
            </Calendar.Grid>
          {/each}
        {/snippet}
      </Calendar.Root>
    {/snippet}

    <div class="custom-range-layout">
      <div class="range-editor">
        <div class="typed-field-group range-editor-row join w-full">
          <label for="custom-start-date" class="typed-field-label join-item">
            Start
          </label>
          <input
            id="custom-start-date"
            type="text"
            placeholder="YYYY-MM-DD"
            class="typed-field input input-sm join-item endpoint-date"
            class:input-error={startInputInvalid}
            aria-invalid={startInputInvalid}
            aria-describedby={startInputInvalid
              ? 'custom-time-range-error'
              : undefined}
            oninput={editStart}
            bind:value={startDateText}
          />
          <input
            id="custom-start-time"
            type="text"
            aria-label="Start time"
            placeholder="HH:mm:ss.SSS"
            class="typed-field input input-sm join-item endpoint-time"
            class:input-error={startInputInvalid}
            aria-invalid={startInputInvalid}
            aria-describedby={startInputInvalid
              ? 'custom-time-range-error'
              : undefined}
            oninput={editStart}
            bind:value={startTimeText}
          />
          <button
            type="button"
            class="endpoint-calendar-button btn btn-circle btn-ghost btn-xs"
            class:endpoint-calendar-button--active={openCalendar === 'start'}
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
          <div
            class="endpoint-calendar-panel"
            transition:slide={{ duration: 120 }}
          >
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
              class="ambiguity-choice"
              class:ambiguity-choice--active={startChoice === 'earlier'}
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
              class="ambiguity-choice"
              class:ambiguity-choice--active={startChoice === 'later'}
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

        <div
          class="typed-field-group range-editor-row range-editor-row--end join w-full"
        >
          <label for="custom-end-date" class="typed-field-label join-item">
            End
          </label>
          <input
            id="custom-end-date"
            type="text"
            placeholder="YYYY-MM-DD"
            class="typed-field input input-sm join-item endpoint-date"
            class:input-error={endInputInvalid}
            aria-invalid={endInputInvalid}
            aria-describedby={endInputInvalid
              ? 'custom-time-range-error'
              : undefined}
            oninput={event => editEndField('date', event.currentTarget.value)}
            bind:value={endDateText}
          />
          <input
            id="custom-end-time"
            type="text"
            aria-label="End time"
            placeholder="right now"
            class="typed-field input input-sm join-item endpoint-time"
            class:input-error={endInputInvalid}
            aria-invalid={endInputInvalid}
            aria-describedby={endInputInvalid
              ? 'custom-time-range-error'
              : undefined}
            oninput={event => editEndField('time', event.currentTarget.value)}
            bind:value={endTimeText}
          />
          <button
            type="button"
            class="endpoint-calendar-button btn btn-circle btn-ghost btn-xs"
            class:endpoint-calendar-button--active={openCalendar === 'end'}
            aria-label="Choose end date"
            aria-expanded={openCalendar === 'end'}
            aria-controls="custom-end-calendar"
            onclick={() =>
              (openCalendar = openCalendar === 'end' ? null : 'end')}
          >
            <HugeiconsIcon icon={Calendar03Icon} size={14} aria-hidden="true" />
          </button>
        </div>

        {#if openCalendar === 'end'}
          <div
            class="endpoint-calendar-panel"
            transition:slide={{ duration: 120 }}
          >
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
              class="ambiguity-choice"
              class:ambiguity-choice--active={endChoice === 'earlier'}
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
              class="ambiguity-choice"
              class:ambiguity-choice--active={endChoice === 'later'}
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

        <div class="custom-range-footer">
          <button
            type="submit"
            class="apply-range btn btn-sm"
            aria-label="Apply range"
          >
            Apply
          </button>
        </div>
      </div>

      {#if customFieldIssue}
        <FieldErrorMessage
          id="custom-time-range-error"
          message={customFieldIssue.message}
        />
      {/if}
    </div>
  </fieldset>
</form>

<style lang="postcss">
  @reference "../../../app.css";

  .custom-range-layout {
    @apply flex min-w-0 w-full flex-col gap-2;
  }

  .range-editor {
    @apply flex min-w-0 w-full flex-col overflow-hidden rounded-xl border border-base-300/70 bg-base-100;
  }

  .range-editor-row {
    @apply rounded-none border-0;
  }

  .range-editor-row--end {
    @apply border-t border-base-300/70;
  }

  .range-editor-row:has(> input:focus) {
    z-index: 1;
    outline: none;
    box-shadow: inset 0 0 0 var(--focus-ring-width) var(--focus-ring-color);
  }

  .range-editor-row:has(> input.input-error:focus) {
    box-shadow: inset 0 0 0 var(--focus-ring-width) var(--color-error);
  }

  .range-editor-row > .typed-field-label {
    @apply rounded-none;
  }

  :global(.custom-calendar) {
    @apply rounded-none border-0 bg-transparent p-2;
  }

  :global(.calendar-header) {
    @apply mb-1 flex items-center justify-between;
  }

  :global(.calendar-heading) {
    @apply text-sm font-medium text-base-content/85;
  }

  :global(.calendar-nav) {
    @apply btn btn-circle btn-ghost btn-xs text-base-content/60 hover:bg-base-300/40 hover:text-base-content;
  }

  :global(.calendar-grid) {
    @apply w-full table-fixed border-collapse select-none;
  }

  :global(.calendar-weekday) {
    @apply h-6 text-center text-[0.625rem] font-medium uppercase tracking-wide text-base-content/45;
  }

  :global(.calendar-cell) {
    @apply h-7 p-0 text-center;
  }

  :global(.calendar-day) {
    @apply relative inline-flex h-7 w-7 items-center justify-center rounded-full border border-transparent bg-transparent p-0 text-xs text-base-content/80;
    @apply hover:bg-base-300/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/35;
  }

  :global(.calendar-day[data-outside-month]) {
    @apply text-base-content/25;
  }

  :global(.calendar-day[data-disabled]) {
    @apply pointer-events-none text-base-content/20;
  }

  :global(.calendar-day[data-selected]) {
    @apply bg-primary text-primary-content;
  }

  .calendar-today-dot {
    @apply absolute bottom-0.5 hidden h-0.5 w-2 rounded-full bg-current opacity-60;
  }

  :global(.calendar-day[data-today]) .calendar-today-dot {
    @apply block;
  }

  .endpoint-date {
    @apply min-w-0 basis-[7.25rem];
  }

  .endpoint-time {
    @apply min-w-0 basis-[8.5rem] border-l border-base-300/70 font-mono tabular-nums;
  }

  .endpoint-calendar-button {
    @apply mr-0.5 h-7 min-h-7 w-7 shrink-0 self-center border-transparent p-0 text-base-content/50 shadow-none;
    @apply hover:bg-base-300/40 hover:text-base-content;
  }

  .endpoint-calendar-button--active {
    @apply border-primary/40 bg-base-300/50 text-primary;
  }

  .endpoint-calendar-panel {
    @apply min-w-0 w-full border-t border-base-300/70 bg-base-200/20;
  }

  .typed-field-group > .endpoint-time:last-child {
    @apply rounded-r-full;
  }

  .ambiguity-choices {
    @apply grid grid-cols-2 gap-1 border-t border-base-300/70 px-2 py-1.5;
  }

  .ambiguity-choice {
    @apply min-w-0 truncate rounded-full border border-base-300 bg-transparent px-2 py-1 font-mono text-[0.625rem] text-base-content/60;
    @apply hover:bg-base-300/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/35;
  }

  .ambiguity-choice--active {
    @apply border-primary/50 bg-primary/10 text-primary;
  }

  .custom-range-footer {
    @apply flex min-w-0 items-center justify-end border-t border-primary/15;
  }

  .apply-range {
    @apply btn-soft btn-primary w-full rounded-none border-0 bg-primary/10 px-4 text-primary shadow-none;
  }

  .apply-range:hover {
    @apply border-0 bg-primary/15 text-primary;
  }

  .apply-range:focus-visible {
    outline-offset: -2px;
  }
</style>
