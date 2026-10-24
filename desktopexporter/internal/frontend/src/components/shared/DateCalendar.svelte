<script lang="ts">
  import { Calendar } from 'bits-ui'
  import type { DateValue } from '@internationalized/date'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { ArrowLeft01Icon, ArrowRight01Icon } from '@hugeicons/core-free-icons'

  let {
    id,
    label,
    value,
    placeholder,
    maxValue,
    onValueChange,
  }: {
    id: string
    label: string
    value?: DateValue
    placeholder: DateValue
    maxValue?: DateValue
    onValueChange: (value: DateValue | undefined) => void
  } = $props()
</script>

<Calendar.Root
  {id}
  type="single"
  class="date-calendar"
  {value}
  {onValueChange}
  {placeholder}
  {maxValue}
  preventDeselect={true}
  initialFocus={true}
  calendarLabel={label}
  weekdayFormat="short"
  fixedWeeks={true}
>
  {#snippet children({ months, weekdays })}
    <Calendar.Header class="calendar-header">
      <Calendar.PrevButton class="calendar-nav">
        <HugeiconsIcon icon={ArrowLeft01Icon} size={14} aria-hidden="true" />
      </Calendar.PrevButton>
      <Calendar.Heading class="calendar-heading" />
      <Calendar.NextButton class="calendar-nav">
        <HugeiconsIcon icon={ArrowRight01Icon} size={14} aria-hidden="true" />
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
                <Calendar.Cell {date} month={month.value} class="calendar-cell">
                  <Calendar.Day class="calendar-day item-state">
                    <span class="calendar-today-dot" aria-hidden="true"></span>
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

<style lang="postcss">
  @reference "../../app.css";

  :global(.date-calendar) {
    @apply rounded-none border-0 bg-transparent px-0 py-4;
  }

  :global(.date-calendar .calendar-header) {
    @apply mb-2 flex items-center justify-center gap-3;
  }

  :global(.date-calendar .calendar-heading) {
    @apply text-sm font-medium text-(--color-subtle);
  }

  :global(.date-calendar .calendar-nav) {
    @apply btn btn-circle btn-ghost btn-xs h-7 min-h-0 w-7 p-0 text-(--color-subtle) hover:bg-base-300/40 hover:text-base-content;
  }

  :global(.date-calendar .calendar-grid) {
    @apply w-full table-fixed border-collapse select-none;
  }

  :global(.date-calendar .calendar-weekday) {
    @apply h-8 text-center text-base font-medium uppercase tracking-wide text-(--color-subtle);
  }

  :global(.date-calendar .calendar-cell) {
    @apply h-7 p-0 text-center;
  }

  :global(.date-calendar .calendar-day) {
    @apply relative inline-flex h-8 w-8 items-center justify-center rounded-full border border-transparent p-0 text-base text-base-content;
  }

  :global(.date-calendar .calendar-day[data-outside-month]) {
    @apply text-(--color-subtle);
  }

  :global(.date-calendar .calendar-day[data-disabled]) {
    @apply pointer-events-none text-base-content/20;
  }

  .calendar-today-dot {
    @apply absolute bottom-0.5 hidden h-0.5 w-2 rounded-full bg-current opacity-60;
  }

  :global(.date-calendar .calendar-day[data-today]) .calendar-today-dot {
    @apply block;
  }
</style>
