<script lang="ts">
  import { tick } from 'svelte'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { ChevronDownIcon } from '@hugeicons/core-free-icons'
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
        <HugeiconsIcon
          icon={ChevronDownIcon}
          size={14}
          strokeWidth={1.5}
          class="timezone-caret"
          aria-hidden="true"
        />
        <span>Timezone</span>
        <span
          class="timezone-summary"
          title={selectedTimezoneLabel}
          aria-live="polite">{selectedTimezoneLabel}</span
        >
      </summary>
      <div class="timezone-content">
        {#if timezoneOpen}
          <div class="timezone-search">
            <input
              id="timezone-filter"
              type="search"
              class="timezone-filter"
              aria-label="Filter timezones"
              placeholder="Filter timezones"
              autocomplete="off"
              spellcheck="false"
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
      </div>
    </details>
  {/snippet}
</CustomTimeRange>

<style lang="postcss">
  @reference "../../../app.css";

  .timezone-heading {
    @apply flex cursor-pointer select-none list-none items-center gap-2 py-1.5 text-sm font-medium;
    color: var(--color-subtle);
  }

  .timezone-heading::marker,
  .timezone-heading::-webkit-details-marker {
    display: none;
  }

  .timezone-heading :global(.timezone-caret) {
    @apply h-7 w-7 shrink-0 p-[7px] transition-transform duration-150;
    color: var(--color-muted);
    transform: rotate(-90deg);
  }

  .timezone-group[open] .timezone-heading :global(.timezone-caret) {
    transform: rotate(0deg);
  }

  .timezone-content {
    @apply pb-2;
  }

  .tz-option {
    box-sizing: border-box;
    height: var(--table-row-h);
    min-height: var(--table-row-h);
    @apply flex w-full cursor-pointer items-center gap-2 rounded-none border-none bg-transparent px-3 py-0 text-left text-sm transition-colors;
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

  .timezone-filter {
    @apply h-8 min-w-0 w-full rounded-full border-0 bg-base-300 px-3.5 py-0 font-mono text-sm tracking-tight text-base-content outline-none;
  }

  .timezone-filter::placeholder {
    color: var(--color-muted);
    opacity: 1;
  }

  .timezone-summary {
    @apply ml-auto min-w-0 truncate pr-3 text-right font-mono text-xs text-base-content;
  }

  .timezone-list__separator {
    @apply border-t border-base-300/50;
  }

  .timezone-list__empty {
    @apply py-3 text-xs text-base-content/55;
  }
</style>
