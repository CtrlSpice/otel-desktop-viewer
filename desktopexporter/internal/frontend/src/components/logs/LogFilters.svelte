<script lang="ts">
  import type { FieldDefinition } from '@/search/model'
  import type { SearchEditorAPI } from '@/components/shared/Search/search-editor-api'
  import FilterPopover from '@/components/shared/Toolbar/FilterPopover.svelte'
  import FilterOption from '@/components/shared/Toolbar/FilterOption.svelte'
  import ListFilter from '@/components/shared/Search/ListFilter.svelte'
  import FieldGroup from '@/components/shared/FieldGroup.svelte'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { FilterIcon } from '@hugeicons/core-free-icons'
  import { severityColor } from './log-severity'
  import { traceServiceColor } from '@/utils/trace-service-color'
  import { themeSignal } from '@/state/theme.svelte'
  import {
    LOG_SEVERITIES,
    readLogFilters,
    setLogFilter,
    type LogFilter,
  } from './log-filter-query'

  let {
    query,
    appliedQuery = query,
    fields,
    services,
    editor,
  }: {
    query: string
    appliedQuery?: string
    fields: FieldDefinition[]
    services: string[]
    editor: SearchEditorAPI | null
  } = $props()
  let serviceSearch = $state('')
  let draftQuery = $state('')
  let filters = $derived(readLogFilters(draftQuery, fields))
  let appliedFilters = $derived(readLogFilters(appliedQuery, fields))
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

  function change(kind: LogFilter, value: string, selected: boolean) {
    const values = selected
      ? [...filters[kind].values, value]
      : filters[kind].values.filter(item => item !== value)
    apply(kind, values)
  }
  function apply(kind: LogFilter, values: string[]) {
    const next = setLogFilter(draftQuery, fields, kind, values)
    if (next !== null) draftQuery = next
  }
  function clearFilters() {
    let next = draftQuery
    for (const kind of ['service', 'severity'] as const) {
      if (!filters[kind].custom)
        next = setLogFilter(next, fields, kind, []) ?? next
    }
    draftQuery = next
  }
</script>

<FilterPopover
  label="Filters"
  popoverClass="filter-popover--choices"
  compact
  showHeader={false}
  showCount
  count={appliedFilters.service.values.length +
    appliedFilters.severity.values.length}
  custom={appliedFilters.editable &&
    (appliedFilters.service.custom || appliedFilters.severity.custom)}
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
      disabled={filters.service.values.length +
        filters.severity.values.length ===
        0}
      onclick={clearFilters}>Clear</button
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
  {#if !filters.editable}<p class="log-filters__hint mb-2" role="status">
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
        {#if filters.service.custom && filters.editable}<p
            class="log-filters__hint"
          >
            Edit service in search
          </p>{/if}
        <div class="filter-options">
          {#each visibleServices as service (service)}
            <FilterOption
              label={`Service ${service}`}
              selected={filters.service.values.includes(service)}
              disabled={filters.service.custom || !editor?.setQuery}
              onclick={() =>
                change(
                  'service',
                  service,
                  !filters.service.values.includes(service)
                )}
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
          {:else}<p class="log-filters__hint">
              {serviceSearch
                ? 'No matching services.'
                : 'No named services in the loaded logs.'}
            </p>{/each}
        </div>
      </FieldGroup>
    {/snippet}
  </ListFilter>
  <FieldGroup label="Severity" bare>
    {#if filters.severity.custom && filters.editable}<p
        class="log-filters__hint"
      >
        Edit severity in search
      </p>{/if}
    <div class="filter-options">
      {#each LOG_SEVERITIES as level (level.label)}
        <FilterOption
          label={`Severity ${level.label}`}
          selected={filters.severity.values.includes(level.label)}
          disabled={filters.severity.custom || !editor?.setQuery}
          onclick={() =>
            change(
              'severity',
              level.label,
              !filters.severity.values.includes(level.label)
            )}
        >
          <span
            class="filter-dot"
            style:background-color={severityColor(level.min)}
            aria-hidden="true"
          ></span>
          <span>{level.label}</span>
        </FilterOption>
      {/each}
    </div>
  </FieldGroup>
</FilterPopover>

<style lang="postcss">
  @reference "../../app.css";
  .log-filters__hint {
    @apply px-2 text-sm;
    color: var(--color-subtle);
  }
</style>
