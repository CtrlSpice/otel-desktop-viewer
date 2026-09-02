<script lang="ts">
  import { tick } from 'svelte'
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
  import { FilterIcon, GlobalIcon, CustomizeIcon } from '@/icons'
  import FieldGroup from '@/components/shared/FieldGroup.svelte'
  import CustomTimeRange from './CustomTimeRange.svelte'
  import RecentTimeRanges from './RecentTimeRanges.svelte'

  let ctx = getTimeContext()
  if (!ctx) {
    throw new Error(
      'Time context not found. Ensure createTimeContext() runs at app root.'
    )
  }

  function normalizeTimezoneSearch(value: string): string {
    return value
      .toLocaleLowerCase()
      .replace(/[_/+\-]/g, ' ')
      .replace(/\s+/g, ' ')
      .trim()
  }

  const localTimezone = resolveTimezoneName('local')
  const namedTimezones = getSupportedTimezones()
    .filter(timezone => timezone !== localTimezone)
    .map(timezone => ({ name: timezone }))
  let timezoneOpen = $state(false)
  let timezoneSearch = $state('')
  let timezoneList: HTMLDivElement | undefined = $state()
  let timezoneReferenceDate = $state(new Date())
  let historicalAbbreviationSearch = $state(new Map<string, string>())
  let historicalAbbreviationLoad: Promise<void> | undefined
  let timezoneOptions = $derived(
    namedTimezones.map(timezone => ({
      ...timezone,
      shortLabel: formatTimezoneLabel(timezone.name, timezoneReferenceDate),
      searchText: normalizeTimezoneSearch(
        `${timezone.name} ${historicalAbbreviationSearch.get(timezone.name) ?? ''}`
      ),
    }))
  )
  let filteredTimezones = $derived(
    timezoneOptions.filter(timezone =>
      `${timezone.searchText} ${normalizeTimezoneSearch(timezone.shortLabel)}`.includes(
        normalizeTimezoneSearch(timezoneSearch)
      )
    )
  )
  let selectedTimezoneLabel = $derived(
    ctx.tz === 'local' || ctx.tz === 'UTC'
      ? formatTimezoneLabel(ctx.tz)
      : ctx.tz
  )

  function loadHistoricalAbbreviations(): Promise<void> {
    historicalAbbreviationLoad ??= import('moment-timezone').then(
      ({ default: moment }) => {
        historicalAbbreviationSearch = new Map(
          namedTimezones.map(({ name }) => [
            name,
            [...new Set(moment.tz.zone(name)?.abbrs ?? [])].join(' '),
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
    const { end } = selectionToQueryRangeMs(ctx.selection, Date.now())
    timezoneReferenceDate = new Date(end)
    tick().then(() => {
      timezoneList
        ?.querySelector<HTMLElement>('[aria-pressed="true"]')
        ?.scrollIntoView?.({ block: 'center', inline: 'nearest' })
    })
  })
</script>

<!-- Shared body: custom range, timezone group, recents group. -->
<div class="flex min-w-0 flex-col text-sm">
  <FieldGroup label="Custom Range">
    {#snippet heading()}
      <CustomizeIcon class="h-3.5 w-3.5 shrink-0 text-base-content/55" />
      <span>Custom Range</span>
    {/snippet}
    <CustomTimeRange />
  </FieldGroup>

  <FieldGroup label="Timezone" bind:open={timezoneOpen}>
    {#snippet heading()}
      <GlobalIcon class="h-3.5 w-3.5 shrink-0 text-base-content/55" />
      <span>Timezone</span>
      <span
        class="timezone-summary"
        title={selectedTimezoneLabel}
        aria-live="polite">{selectedTimezoneLabel}</span
      >
    {/snippet}
    {#if timezoneOpen}
      <div class="timezone-search">
        <div class="typed-field-group join w-full">
          <label
            for="timezone-filter"
            class="typed-field-label join-item"
            title="Filter timezones"
          >
            <FilterIcon class="h-3.5 w-3.5" />
            <span class="sr-only">Filter timezones</span>
          </label>
          <input
            id="timezone-filter"
            type="search"
            class="typed-field input input-sm join-item"
            placeholder="Filter timezones"
            autocomplete="off"
            spellcheck="false"
            bind:value={timezoneSearch}
          />
        </div>
      </div>
      <div
        class="timezone-list"
        aria-label="Timezone options"
        bind:this={timezoneList}
      >
        <button
          type="button"
          class="tz-option"
          class:tz-option--active={ctx.tz === 'local'}
          aria-pressed={ctx.tz === 'local'}
          onclick={() => ctx.setTz('local')}
        >
          <!-- "(Local)" disambiguates from UTC when the machine follows UTC. -->
          <span class="tz-name">{getLocalTimezoneName()} (Local)</span>
          <span class="tz-badge">{formatTimezoneLabel('local')}</span>
        </button>
        <button
          type="button"
          class="tz-option"
          class:tz-option--active={ctx.tz === 'UTC'}
          aria-pressed={ctx.tz === 'UTC'}
          onclick={() => ctx.setTz('UTC')}
        >
          <span class="tz-name">Coordinated Universal Time</span>
          <span class="tz-badge">UTC</span>
        </button>
        <div class="timezone-list__separator" role="separator"></div>
        {#each filteredTimezones as timezone (timezone.name)}
          <button
            type="button"
            class="tz-option"
            class:tz-option--active={ctx.tz === timezone.name}
            aria-pressed={ctx.tz === timezone.name}
            onclick={() => ctx.setTz(timezone.name)}
          >
            <span class="tz-name">{timezone.name}</span>
            <span class="tz-badge">{timezone.shortLabel}</span>
          </button>
        {/each}
        {#if filteredTimezones.length === 0}
          <p class="timezone-list__empty">No matching named timezones</p>
        {/if}
      </div>
    {/if}
  </FieldGroup>

  <RecentTimeRanges />
</div>

<style lang="postcss">
  @reference "../../../app.css";

  .tz-option {
    box-sizing: border-box;
    height: var(--table-row-h);
    min-height: var(--table-row-h);
    @apply flex w-full cursor-pointer items-center gap-2 rounded-none border-none bg-transparent px-0 py-0 text-left text-sm transition-colors;
    @apply text-base-content/90 hover:bg-base-300/40;
    @apply focus-visible:bg-base-300/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/30 focus-visible:ring-offset-0;
  }

  .tz-option--active {
    @apply text-primary;
  }

  .tz-name {
    @apply min-w-0 flex-1 truncate text-sm;
  }

  .tz-badge {
    @apply ml-auto shrink-0 font-mono text-xs text-base-content/55 tabular-nums;
  }

  .timezone-list {
    @apply max-h-72 overflow-y-auto;
    scrollbar-width: thin;
  }

  .timezone-search {
    @apply py-1.5;
  }

  .timezone-summary {
    @apply ml-auto max-w-40 truncate font-mono text-xs text-base-content/55;
  }

  .timezone-list__separator {
    @apply border-t border-base-300/50;
  }

  .timezone-list__empty {
    @apply py-3 text-xs text-base-content/55;
  }
</style>
