<script lang="ts">
  import { onMount, untrack } from 'svelte'
  import { RangeCalendar } from 'bits-ui'
  import {
    CalendarDate,
    parseDate,
    type DateValue,
  } from '@internationalized/date'
  import FieldErrorMessage from '@/components/shared/FieldErrorMessage.svelte'
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import { ArrowLeftIcon, ArrowRightIcon, CheckmarkCircleIcon } from '@/icons'
  import {
    formatDateTime,
    formatEditableDateTime,
    formatTimezoneLabel,
    parseDateTimeInTimezone,
    resolveTimezoneName,
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
  type CalendarRange = {
    start: DateValue | undefined
    end: DateValue | undefined
  }
  type Ambiguity = { earlier: number; later: number }

  const DATE_PATTERN = /^\d{4}-\d{2}-\d{2}$/
  const TIME_PATTERN = /^(?:[01]\d|2[0-3]):[0-5]\d:[0-5]\d\.\d{3}$/

  let startDateText = $state('')
  let startTimeText = $state('00:00:00.000')
  let endDateText = $state('')
  let endTimeText = $state('00:00:00.000')
  let endIsNow = $state(true)
  let nowTimestamp = $state(Date.now())
  let calendarValue = $state<CalendarRange>({
    start: undefined,
    end: undefined,
  })
  let startChoice = $state<Choice | null>(null)
  let endChoice = $state<Choice | null>(null)
  let startAmbiguity = $state<Ambiguity | null>(null)
  let endAmbiguity = $state<Ambiguity | null>(null)
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

  let today = $derived(calendarDateForTimestamp(nowTimestamp))
  let nowPreview = $derived(
    `Now · ${formatDateTime(nowTimestamp, ctx.tz, 'milliseconds')}`
  )

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

  function useNow() {
    nowTimestamp = Date.now()
    endIsNow = true
    endChoice = null
    endAmbiguity = null
    customFieldIssue = null
  }

  function editEndpoint(endpoint: Endpoint) {
    if (endpoint === 'start') {
      startChoice = null
      startAmbiguity = null
    } else {
      endIsNow = false
      endChoice = null
      endAmbiguity = null
    }
    customFieldIssue = null
  }

  function setCalendarRange(value: {
    start: DateValue | undefined
    end: DateValue | undefined
  }) {
    calendarValue = value
    if (value.start) {
      startDateText = value.start.toString()
      startChoice = null
      startAmbiguity = null
    }
    if (value.end) {
      endDateText = value.end.toString()
      endIsNow = false
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
    if (ctx.selection.type === 'custom') {
      const start = editableParts(ctx.selection.start)
      const end = editableParts(ctx.selection.end)
      const initializedStart = initializeChoice(
        start.date,
        start.time,
        ctx.selection.start
      )
      const initializedEnd = initializeChoice(
        end.date,
        end.time,
        ctx.selection.end
      )
      startDateText = start.date
      startTimeText = start.time
      endDateText = end.date
      endTimeText = end.time
      endIsNow = false
      startChoice = initializedStart.choice
      endChoice = initializedEnd.choice
      startAmbiguity = initializedStart.ambiguity
      endAmbiguity = initializedEnd.ambiguity
      calendarValue = {
        start: calendarDate(start.date),
        end: calendarDate(end.date),
      }
    } else {
      nowTimestamp = Date.now()
      startDateText = ''
      startTimeText = '00:00:00.000'
      endIsNow = true
      calendarValue = { start: undefined, end: undefined }
      startChoice = null
      endChoice = null
      startAmbiguity = null
      endAmbiguity = null
    }
    customFieldIssue = null
  })

  $effect(() => {
    if (!endIsNow) return
    const end = editableParts(nowTimestamp)
    endDateText = end.date
    endTimeText = end.time
  })

  onMount(() => {
    const interval = window.setInterval(() => {
      nowTimestamp = Date.now()
    }, 1000)
    return () => window.clearInterval(interval)
  })

  function validateCustomRange(): ValidationResult {
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

    const endResult: EndpointResult = endIsNow
      ? { isValid: true, timestamp: Date.now() }
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

    if (endResult.timestamp > Date.now()) {
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

    <div class="custom-range-layout">
      <RangeCalendar.Root
        class="custom-calendar"
        value={calendarValue}
        onValueChange={setCalendarRange}
        placeholder={calendarValue.start ?? calendarValue.end ?? today}
        maxValue={today}
        calendarLabel="Custom time range"
        weekdayFormat="short"
        fixedWeeks={true}
      >
        {#snippet children({ months, weekdays })}
          <RangeCalendar.Header class="calendar-header">
            <RangeCalendar.PrevButton class="calendar-nav">
              <ArrowLeftIcon class="h-3.5 w-3.5" />
            </RangeCalendar.PrevButton>
            <RangeCalendar.Heading class="calendar-heading" />
            <RangeCalendar.NextButton class="calendar-nav">
              <ArrowRightIcon class="h-3.5 w-3.5" />
            </RangeCalendar.NextButton>
          </RangeCalendar.Header>

          {#each months as month (month.value.toString())}
            <RangeCalendar.Grid class="calendar-grid">
              <RangeCalendar.GridHead>
                <RangeCalendar.GridRow>
                  {#each weekdays as weekday (weekday)}
                    <RangeCalendar.HeadCell class="calendar-weekday">
                      {weekday.slice(0, 2)}
                    </RangeCalendar.HeadCell>
                  {/each}
                </RangeCalendar.GridRow>
              </RangeCalendar.GridHead>
              <RangeCalendar.GridBody>
                {#each month.weeks as weekDates, week (week)}
                  <RangeCalendar.GridRow>
                    {#each weekDates as date (date.toString())}
                      <RangeCalendar.Cell
                        {date}
                        month={month.value}
                        class="calendar-cell"
                      >
                        <RangeCalendar.Day class="calendar-day">
                          <span class="calendar-today-dot" aria-hidden="true"
                          ></span>
                          {date.day}
                        </RangeCalendar.Day>
                      </RangeCalendar.Cell>
                    {/each}
                  </RangeCalendar.GridRow>
                {/each}
              </RangeCalendar.GridBody>
            </RangeCalendar.Grid>
          {/each}
        {/snippet}
      </RangeCalendar.Root>

      <div class="typed-field-group join w-full">
        <label for="custom-start-date" class="typed-field-label join-item">
          Start
        </label>
        <input
          id="custom-start-date"
          type="text"
          inputmode="numeric"
          placeholder="YYYY-MM-DD"
          class="typed-field input input-sm join-item endpoint-date"
          class:input-error={startInputInvalid}
          aria-invalid={startInputInvalid}
          aria-describedby={customFieldIssue
            ? 'custom-time-range-error'
            : undefined}
          oninput={() => editEndpoint('start')}
          bind:value={startDateText}
        />
        <input
          id="custom-start-time"
          type="text"
          inputmode="decimal"
          aria-label="Start time"
          placeholder="HH:mm:ss.SSS"
          class="typed-field input input-sm join-item endpoint-time"
          class:input-error={startInputInvalid}
          aria-invalid={startInputInvalid}
          aria-describedby={customFieldIssue
            ? 'custom-time-range-error'
            : undefined}
          oninput={() => editEndpoint('start')}
          bind:value={startTimeText}
        />
      </div>

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
              startChoice = 'later'
              customFieldIssue = null
            }}
          >
            {ambiguityLabel(startAmbiguity.later, 'later')}
          </button>
        </div>
      {/if}

      <div class="typed-field-group join w-full">
        <label for="custom-end-date" class="typed-field-label join-item">
          End
        </label>
        <input
          id="custom-end-date"
          type="text"
          inputmode="numeric"
          placeholder="YYYY-MM-DD"
          class="typed-field input input-sm join-item endpoint-date"
          class:input-error={endInputInvalid}
          aria-invalid={endInputInvalid}
          aria-describedby={customFieldIssue
            ? 'custom-time-range-error'
            : undefined}
          onfocus={() => editEndpoint('end')}
          oninput={() => editEndpoint('end')}
          bind:value={endDateText}
        />
        <input
          id="custom-end-time"
          type="text"
          inputmode="decimal"
          aria-label="End time"
          placeholder="HH:mm:ss.SSS"
          class="typed-field input input-sm join-item endpoint-time"
          class:input-error={endInputInvalid}
          aria-invalid={endInputInvalid}
          aria-describedby={customFieldIssue
            ? 'custom-time-range-error'
            : undefined}
          onfocus={() => editEndpoint('end')}
          oninput={() => editEndpoint('end')}
          bind:value={endTimeText}
        />
        <button
          type="button"
          class="typed-field typed-field--action now-button btn btn-sm join-item"
          class:now-button--active={endIsNow}
          aria-pressed={endIsNow}
          onclick={useNow}
        >
          Now
        </button>
      </div>

      {#if endIsNow}
        <p class="now-preview" aria-live="off">{nowPreview}</p>
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
              endChoice = 'later'
              customFieldIssue = null
            }}
          >
            {ambiguityLabel(endAmbiguity.later, 'later')}
          </button>
        </div>
      {/if}

      {#if customFieldIssue}
        <FieldErrorMessage
          id="custom-time-range-error"
          message={customFieldIssue.message}
        />
      {/if}

      <div class="custom-range-footer">
        <span class="timezone-context" title={resolveTimezoneName(ctx.tz)}>
          Times use {resolveTimezoneName(ctx.tz)}
        </span>
        <button type="submit" class="apply-range btn btn-sm">
          <CheckmarkCircleIcon class="h-3.5 w-3.5" />
          Apply range
        </button>
      </div>
    </div>
  </fieldset>
</form>

<style lang="postcss">
  @reference "../../../app.css";

  .custom-range-layout {
    @apply flex min-w-0 w-full flex-col gap-2;
  }

  :global(.custom-calendar) {
    @apply rounded-lg border border-base-300/70 bg-base-100 p-2;
  }

  :global(.calendar-header) {
    @apply mb-1 flex items-center justify-between;
  }

  :global(.calendar-heading) {
    @apply text-sm font-medium text-base-content/85;
  }

  :global(.calendar-nav) {
    @apply btn btn-ghost btn-xs btn-square text-base-content/60 hover:bg-base-300/40 hover:text-base-content;
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
    @apply relative inline-flex h-7 w-full items-center justify-center rounded-sm border border-transparent bg-transparent p-0 text-xs text-base-content/80;
    @apply hover:bg-base-300/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/35;
  }

  :global(.calendar-day[data-outside-month]) {
    @apply text-base-content/25;
  }

  :global(.calendar-day[data-disabled]) {
    @apply pointer-events-none text-base-content/20;
  }

  :global(.calendar-day[data-range-middle]) {
    @apply rounded-none bg-primary/10 text-base-content;
  }

  :global(.calendar-day[data-selection-start]),
  :global(.calendar-day[data-selection-end]),
  :global(.calendar-day[data-range-start]),
  :global(.calendar-day[data-range-end]) {
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

  .typed-field-group > .endpoint-time:last-child {
    @apply rounded-r-full;
  }

  .now-button {
    @apply px-3 font-sans font-medium text-base-content/60;
  }

  .now-button--active {
    @apply bg-primary text-primary-content;
  }

  .now-preview {
    @apply truncate px-3 font-mono text-[0.6875rem] text-base-content/55 tabular-nums;
  }

  .ambiguity-choices {
    @apply grid grid-cols-2 gap-1 px-1;
  }

  .ambiguity-choice {
    @apply min-w-0 truncate rounded-full border border-base-300 bg-transparent px-2 py-1 font-mono text-[0.625rem] text-base-content/60;
    @apply hover:bg-base-300/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/35;
  }

  .ambiguity-choice--active {
    @apply border-primary/50 bg-primary/10 text-primary;
  }

  .custom-range-footer {
    @apply flex min-w-0 items-center justify-between gap-2 pt-1;
  }

  .timezone-context {
    @apply min-w-0 truncate text-[0.6875rem] text-base-content/50;
  }

  .apply-range {
    @apply shrink-0 rounded-full border-primary/30 bg-primary/10 px-4 text-primary shadow-none hover:bg-primary hover:text-primary-content;
  }
</style>
