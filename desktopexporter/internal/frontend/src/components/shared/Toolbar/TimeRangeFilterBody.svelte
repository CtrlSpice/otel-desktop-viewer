<script lang="ts">
  import { tick } from 'svelte'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { Globe02Icon } from '@hugeicons/core-free-icons'
  import FilterInput from '@/components/shared/Search/FilterInput.svelte'
  import {
    getTimeContext,
    selectionToQueryRangeMs,
  } from '@/contexts/time-context.svelte'
  import {
    formatTimezoneLabel,
    getLocalTimezoneName,
    getSupportedTimezones,
    resolveTimezoneName,
  } from '@/utils/time'
  import CustomTimeRange from './CustomTimeRange.svelte'

  const ctx = getTimeContext()

  function normalizeTimezoneSearch(value: string): string {
    return value
      .toLocaleLowerCase()
      .replace(/[_/+-]/g, ' ')
      .replace(/\s+/g, ' ')
      .trim()
  }

  const localTimezone = resolveTimezoneName('local')
  const timezoneChoices = [
    {
      name: 'local' as const,
      zone: localTimezone,
      label: `${getLocalTimezoneName()} (Local)`,
    },
    { name: 'UTC' as const, zone: 'UTC', label: 'Coordinated Universal Time' },
    ...getSupportedTimezones()
      .filter(timezone => timezone !== localTimezone)
      .map(name => ({ name, zone: name, label: name })),
  ].sort((a, b) => a.label.localeCompare(b.label))
  let timezoneOpen = $state(false)
  let timezoneSearch = $state('')
  let timezoneList: HTMLDivElement | undefined = $state()
  let timezoneReferenceDate = $state(new Date())
  let historicalAbbreviationSearch = $state(new Map<string, string>())
  let historicalAbbreviationLoad: Promise<void> | undefined
  let timezoneOptions = $derived(
    timezoneChoices.map(timezone => {
      const shortLabel = formatTimezoneLabel(
        timezone.name,
        timezoneReferenceDate
      )
      const longLabel = new Intl.DateTimeFormat('en', {
        timeZone: timezone.zone,
        timeZoneName: 'long',
      })
        .formatToParts(timezoneReferenceDate)
        .find(part => part.type === 'timeZoneName')?.value
      return {
        ...timezone,
        shortLabel,
        searchText: normalizeTimezoneSearch(
          [
            timezone.name,
            timezone.zone,
            timezone.label,
            shortLabel,
            longLabel,
            historicalAbbreviationSearch.get(timezone.zone),
          ].join(' ')
        ),
      }
    })
  )
  let filteredTimezones = $derived(
    timezoneOptions.filter(timezone =>
      timezone.searchText.includes(normalizeTimezoneSearch(timezoneSearch))
    )
  )
  let selectedTimezoneLabel = $derived(
    timezoneOptions.find(timezone => timezone.name === ctx.tz)?.label ?? ctx.tz
  )

  function loadHistoricalAbbreviations(): Promise<void> {
    historicalAbbreviationLoad ??= import('moment-timezone').then(
      ({ default: moment }) => {
        historicalAbbreviationSearch = new Map(
          timezoneChoices.map(({ zone }) => [
            zone,
            [...new Set(moment.tz.zone(zone)?.abbrs ?? [])].join(' '),
          ])
        )
      }
    )
    return historicalAbbreviationLoad
  }

  $effect(() => {
    if (!timezoneOpen) return
    void loadHistoricalAbbreviations()
    timezoneSearch = ''
    const now = Date.now()
    const { endTime } = selectionToQueryRangeMs(ctx.selection, now)
    timezoneReferenceDate = new Date(endTime ?? now)
    tick().then(() => {
      timezoneList
        ?.querySelector<HTMLElement>('[aria-pressed="true"]')
        ?.scrollIntoView?.({ block: 'center', inline: 'nearest' })
    })
  })
</script>

<!-- Shared body: always-visible custom range and timezone group. -->
<CustomTimeRange>
  {#snippet beforeApply()}
    <details
      class="timezone-group"
      open={timezoneOpen}
      ontoggle={event => (timezoneOpen = event.currentTarget.open)}
    >
      <summary class="timezone-heading">
        <span>Timezone:</span>
        <span
          class="timezone-summary"
          title={selectedTimezoneLabel}
          aria-live="polite">{selectedTimezoneLabel}</span
        >
        <HugeiconsIcon
          icon={Globe02Icon}
          size={14}
          strokeWidth={1.5}
          class="timezone-icon"
          aria-hidden="true"
        />
      </summary>
      <div class="timezone-content">
        {#if timezoneOpen}
          <div class="timezone-search">
            <FilterInput
              id="timezone-filter"
              label="Filter timezones"
              onkeydown={event => {
                if (event.key === 'Enter') event.preventDefault()
              }}
              bind:value={timezoneSearch}
            />
          </div>
          <div
            class="timezone-list"
            aria-label="Timezone options"
            bind:this={timezoneList}
          >
            {#each filteredTimezones as timezone (timezone.name)}
              <button
                type="button"
                class="tz-option"
                class:tz-option--active={ctx.tz === timezone.name}
                aria-pressed={ctx.tz === timezone.name}
                onclick={() => ctx.setTz(timezone.name)}
              >
                <span class="tz-name">{timezone.label}</span>
                <span class="tz-badge">{timezone.shortLabel}</span>
              </button>
            {/each}
            {#if filteredTimezones.length === 0}
              <p class="timezone-list__empty">No matching timezones</p>
            {/if}
          </div>
        {/if}
      </div>
    </details>
  {/snippet}
</CustomTimeRange>

<style lang="postcss">
  @reference "../../../app.css";

  .timezone-heading {
    @apply flex cursor-pointer select-none list-none items-center gap-2 py-1.5 pl-3 text-sm font-medium;
    color: var(--color-subtle);
  }

  .timezone-heading::marker,
  .timezone-heading::-webkit-details-marker {
    display: none;
  }

  .timezone-heading :global(.timezone-icon) {
    @apply mr-1 h-7 w-7 shrink-0 p-[7px];
    color: var(--color-muted);
  }

  .timezone-content {
    @apply pb-2;
  }

  .tz-option {
    height: var(--table-row-h);
    @apply flex w-full cursor-pointer items-center gap-2 rounded-none border-none bg-transparent px-3 py-0 text-left text-sm transition-colors;
    @apply text-base-content/90 hover:bg-base-300/40;
    @apply focus-visible:bg-base-300/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/30 focus-visible:ring-offset-0;
  }

  .tz-option--active {
    @apply text-primary;
  }

  .tz-name {
    @apply min-w-0 flex-1 truncate;
  }

  .tz-badge {
    @apply shrink-0 font-mono text-xs text-base-content/55 tabular-nums;
  }

  .timezone-list {
    @apply mt-2 max-h-72 overflow-y-auto;
    scrollbar-width: thin;
  }

  .timezone-search {
    @apply py-1.5;
  }

  .timezone-summary {
    @apply ml-auto min-w-0 truncate text-right font-mono text-xs text-base-content;
  }

  .timezone-list__empty {
    @apply py-3 text-xs text-base-content/55;
  }
</style>
