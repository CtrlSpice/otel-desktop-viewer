<script lang="ts">
  import type { FieldDefinition } from '@/search/model'
  import type { SearchEditorAPI } from '@/components/shared/Search/search-editor-api'
  import FilterPopover from './Toolbar/FilterPopover.svelte'
  import FilterInput from './Search/FilterInput.svelte'
  import FieldGroup from './FieldGroup.svelte'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { FilterIcon } from '@hugeicons/core-free-icons'
  import { METRIC_TYPE_ENUM } from '@/constants/fields'
  import { traceServiceColor } from '@/utils/trace-service-color'
  import { themeSignal } from '@/state/theme.svelte'
  import {
    readListFilters,
    setListFilter,
    type ListFilter,
  } from './list-filter-query'

  let {
    signal,
    query,
    appliedQuery = query,
    fields,
    services,
    editor,
  }: {
    signal: 'traces' | 'metrics'
    query: string
    appliedQuery?: string
    fields: FieldDefinition[]
    services: string[]
    editor: SearchEditorAPI | null
  } = $props()
  let draftQuery = $state('')
  let serviceSearch = $state('')
  let kinds = $derived([
    'service',
    signal === 'traces' ? 'errors' : 'metricType',
  ] as ListFilter[])
  let filters = $derived(readListFilters(draftQuery, fields, signal))
  let applied = $derived(readListFilters(appliedQuery, fields, signal))
  let count = $derived(
    kinds.reduce((sum, kind) => sum + applied[kind].values.length, 0)
  )
  let choices = $derived(
    [...new Set([...services, ...filters.service.values])].sort((a, b) =>
      a.localeCompare(b)
    )
  )
  let visibleServices = $derived(
    choices.filter(
      service =>
        filters.service.values.includes(service) ||
        service.toLowerCase().includes(serviceSearch.toLowerCase())
    )
  )

  function toggle(kind: ListFilter, value: string) {
    const values = filters[kind].values.includes(value)
      ? filters[kind].values.filter(item => item !== value)
      : [...filters[kind].values, value]
    draftQuery =
      setListFilter(draftQuery, fields, kind, values, signal) ?? draftQuery
  }
  function clear() {
    for (const kind of kinds)
      draftQuery =
        setListFilter(draftQuery, fields, kind, [], signal) ?? draftQuery
  }
</script>

<FilterPopover
  label="Filters"
  compact
  showCount
  {count}
  custom={applied.editable && kinds.some(kind => applied[kind].custom)}
  clearable={kinds.some(kind => filters[kind].values.length > 0)}
  onOpenChange={open => {
    if (open) {
      draftQuery = query
      serviceSearch = ''
    }
  }}
  onClear={clear}
>
  {#snippet icon()}<HugeiconsIcon
      icon={FilterIcon}
      size={16}
      strokeWidth={1.5}
      aria-hidden="true"
    />{/snippet}
  {#snippet footer(close)}
    <button
      type="button"
      class="btn btn-ghost btn-sm text-sm font-normal text-primary"
      disabled={!editor?.setQuery || draftQuery === query || !filters.editable}
      onclick={() => {
        editor?.setQuery?.(draftQuery)
        close()
      }}>Apply</button
    >
  {/snippet}
  {#if !filters.editable}<p class="filter-hint">
      Complete the expression in search before changing filters.
    </p>{/if}
  {#if signal === 'traces'}<p class="filter-hint">
      Matches spans, not just roots. Service and Has errors must match the same
      span.
    </p>{/if}
  <FieldGroup label="Service">
    <FilterInput
      label="Find services"
      placeholder="Find services…"
      bind:value={serviceSearch}
    />
    <p class="filter-hint">
      {signal === 'traces'
        ? 'Root services seen in loaded traces; other span services can be entered in search.'
        : 'Services seen in loaded metrics.'}
    </p>
    {#if filters.service.custom && filters.editable}<p class="filter-hint">
        Edit this service condition in search.
      </p>{/if}
    <div class="filter-options">
      {#each visibleServices as service (service)}
        <button
          type="button"
          class="filter-option item-state"
          aria-label={`Service ${service}`}
          aria-pressed={filters.service.values.includes(service)}
          disabled={filters.service.custom || !editor?.setQuery}
          onclick={() => toggle('service', service)}
        >
          <span
            class="filter-dot"
            style:background-color={traceServiceColor(
              service,
              themeSignal.value
            )}
            aria-hidden="true"
          ></span>
          <span class="min-w-0 truncate" title={service}>{service}</span>
        </button>
      {:else}<p class="filter-hint">
          {serviceSearch
            ? 'No matching services.'
            : 'No named services in the loaded records.'}
        </p>{/each}
    </div>
  </FieldGroup>
  {@const kind = signal === 'traces' ? 'errors' : 'metricType'}
  <FieldGroup label={signal === 'traces' ? 'Span status' : 'Type'}>
    {#if filters[kind].custom && filters.editable}<p class="filter-hint">
        Edit this condition in search.
      </p>{/if}
    <div class="filter-options">
      {#each signal === 'traces' ? ['Has errors'] : METRIC_TYPE_ENUM as value (value)}
        <button
          type="button"
          class="filter-option item-state"
          aria-pressed={filters[kind].values.includes(value)}
          disabled={filters[kind].custom || !editor?.setQuery}
          onclick={() => toggle(kind, value)}
        >
          {#if signal === 'traces'}<span
              class="filter-dot bg-error"
              aria-hidden="true"
            ></span>{/if}
          {value === 'ExponentialHistogram' ? 'Exp. Histogram' : value}
        </button>
      {/each}
    </div>
  </FieldGroup>
</FilterPopover>

<style lang="postcss">
  @reference "../../app.css";
  .filter-options {
    @apply flex min-w-0 flex-wrap gap-2 px-2 py-1;
  }
  .filter-option {
    @apply inline-flex min-h-8 max-w-full cursor-pointer items-center gap-2 border-0 px-3 py-1 text-sm font-normal text-base-content;
  }
  .filter-option:disabled {
    @apply cursor-default opacity-50;
  }
  .filter-dot {
    @apply size-2 shrink-0 rounded-full;
  }
  .filter-hint {
    @apply my-2 px-2 text-sm;
    color: var(--color-subtle);
  }
</style>
