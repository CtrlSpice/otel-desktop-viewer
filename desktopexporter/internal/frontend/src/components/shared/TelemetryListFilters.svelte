<script lang="ts">
  import type { FieldDefinition } from '@/search/model'
  import type { SearchEditorAPI } from '@/components/shared/Search/search-editor-api'
  import FilterPopover from './Toolbar/FilterPopover.svelte'
  import FilterOption from './Toolbar/FilterOption.svelte'
  import ListFilter from './Search/ListFilter.svelte'
  import FieldGroup from './FieldGroup.svelte'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import {
    FilterIcon,
    GaugeIcon,
    SigmaIcon,
    ChartHistogramIcon,
    ChartIncreaseIcon,
    CircleIcon,
  } from '@hugeicons/core-free-icons'
  import { METRIC_TYPE_ENUM } from '@/constants/fields'
  import { traceServiceColor } from '@/utils/trace-service-color'
  import { themeSignal } from '@/state/theme.svelte'
  import {
    readListFilters,
    setListFilter,
    type ListFilter as ListFilterKind,
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
  const metricTypeIcons = {
    Gauge: GaugeIcon,
    Sum: SigmaIcon,
    Histogram: ChartHistogramIcon,
    ExponentialHistogram: ChartIncreaseIcon,
    Empty: CircleIcon,
  }
  let serviceSearch = $state('')
  let kinds: ListFilterKind[] = $derived([
    'service',
    signal === 'traces' ? 'errors' : 'metricType',
  ])
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

  function toggle(kind: ListFilterKind, value: string) {
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
  popoverClass="filter-popover--choices"
  compact
  showHeader={false}
  showCount
  {count}
  custom={applied.editable && kinds.some(kind => applied[kind].custom)}
  onOpenChange={open => {
    if (open) {
      draftQuery = query
      serviceSearch = ''
    }
  }}
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
      class="btn btn-ghost btn-sm text-sm font-normal"
      aria-label="Clear filters"
      disabled={!kinds.some(kind => filters[kind].values.length > 0)}
      onclick={clear}>Clear</button
    >
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
  {#if !filters.editable}<p class="filter-hint" role="status">
      Edit query in search
    </p>{/if}
  <ListFilter
    label="Find services"
    placeholder="Find services…"
    bind:value={serviceSearch}
  >
    {#snippet children(filterToggle, filterField)}
      <FieldGroup label="Service" bare>
        {#snippet headerAction()}
          <span class="min-w-0 flex-1">Service</span>
          {@render filterToggle()}
        {/snippet}
        {@render filterField()}
        {#if filters.service.custom && filters.editable}<p class="filter-hint">
            Edit service in search
          </p>{/if}
        <div class="filter-options">
          {#each visibleServices as service (service)}
            <FilterOption
              label={`Service ${service}`}
              selected={filters.service.values.includes(service)}
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
            </FilterOption>
          {:else}<p class="filter-hint">
              {serviceSearch
                ? 'No matching services.'
                : 'No named services in the loaded records.'}
            </p>{/each}
        </div>
      </FieldGroup>
    {/snippet}
  </ListFilter>
  {@const kind = signal === 'traces' ? 'errors' : 'metricType'}
  <FieldGroup label={signal === 'traces' ? 'Span status' : 'Type'} bare>
    {#if filters[kind].custom && filters.editable}<p class="filter-hint">
        Edit {signal === 'traces' ? 'status' : 'type'} in search
      </p>{/if}
    <div class="filter-options">
      {#each signal === 'traces' ? ['Has errors'] : METRIC_TYPE_ENUM as value (value)}
        <FilterOption
          selected={filters[kind].values.includes(value)}
          disabled={filters[kind].custom || !editor?.setQuery}
          onclick={() => toggle(kind, value)}
        >
          {#if signal === 'traces'}<span
              class="filter-dot bg-error"
              aria-hidden="true"
            ></span>{:else if value in metricTypeIcons}<HugeiconsIcon
              icon={metricTypeIcons[value as keyof typeof metricTypeIcons]}
              size={16}
              strokeWidth={1.5}
              aria-hidden="true"
            />{/if}
          {value === 'ExponentialHistogram' ? 'Exp. Histogram' : value}
        </FilterOption>
      {/each}
    </div>
  </FieldGroup>
</FilterPopover>

<style lang="postcss">
  @reference "../../app.css";
  .filter-hint {
    @apply my-2 px-2 text-sm;
    color: var(--color-subtle);
  }
</style>
