<script lang="ts">
  import type { FieldDefinition } from '@/search/model'
  import type { SearchEditorAPI } from '@/components/shared/Search/search-editor-api'
  import FilterPopover from '@/components/shared/Toolbar/FilterPopover.svelte'
  import FieldGroup from '@/components/shared/FieldGroup.svelte'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { FilterIcon } from '@hugeicons/core-free-icons'
  import {
    LOG_SEVERITIES,
    readLogFilters,
    setLogFilter,
    type LogFilter,
  } from './log-filter-query'

  let {
    query,
    fields,
    services,
    editor,
  }: {
    query: string
    fields: FieldDefinition[]
    services: string[]
    editor: SearchEditorAPI | null
  } = $props()
  let serviceSearch = $state('')
  let filters = $derived(readLogFilters(query, fields))
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
    const next = setLogFilter(query, fields, kind, values)
    if (next !== null) editor?.setQuery?.(next)
  }
  function clearFilters() {
    let next = query
    for (const kind of ['service', 'severity'] as const) {
      if (!filters[kind].custom)
        next = setLogFilter(next, fields, kind, []) ?? next
    }
    if (next !== query) editor?.setQuery?.(next)
  }
</script>

{#snippet queryHint()}
  {#if !filters.editable}<p class="log-filters__hint mb-2">
      Use a complete field expression in search to combine it with filters.
    </p>{/if}
{/snippet}

<FilterPopover
  label="Filters"
  compact
  count={filters.service.values.length + filters.severity.values.length}
  custom={filters.editable &&
    (filters.service.custom || filters.severity.custom)}
  onClear={clearFilters}
>
  {#snippet icon()}<HugeiconsIcon
      icon={FilterIcon}
      size={16}
      strokeWidth={1.5}
      aria-hidden="true"
    />{/snippet}
  <FieldGroup label="Service">
    {#snippet heading()}
      <span class="flex-1">Service</span>
      {#if filters.service.values.length}<button
          type="button"
          class="btn btn-ghost btn-xs"
          aria-label="Clear service filter"
          onclick={event => {
            event.preventDefault()
            event.stopPropagation()
            apply('service', [])
          }}>Clear</button
        >{/if}
    {/snippet}
    {@render queryHint()}
    <input
      class="typed-field typed-field--pill mb-2 w-full"
      type="search"
      aria-label="Find services"
      placeholder="Find services…"
      bind:value={serviceSearch}
    />
    <p class="log-filters__hint mb-1">Services seen in loaded logs</p>
    {#if filters.service.custom && filters.editable}<p
        class="log-filters__hint"
      >
        Edit this service condition in search.
      </p>{/if}
    {#each visibleServices as service (service)}
      <label class="log-filters__choice item-state">
        <input
          type="checkbox"
          class="checkbox checkbox-sm"
          aria-label={`Service ${service}`}
          checked={filters.service.values.includes(service)}
          disabled={filters.service.custom || !editor?.setQuery}
          onchange={event =>
            change('service', service, event.currentTarget.checked)}
        />
        <span class="min-w-0 truncate" title={service}>{service}</span>
      </label>
    {:else}<p class="log-filters__hint">
        {serviceSearch
          ? 'No matching services.'
          : 'No named services in the loaded logs.'}
      </p>{/each}
  </FieldGroup>
  <FieldGroup label="Severity">
    {#snippet heading()}
      <span class="flex-1">Severity</span>
      {#if filters.severity.values.length}<button
          type="button"
          class="btn btn-ghost btn-xs"
          aria-label="Clear severity filter"
          onclick={event => {
            event.preventDefault()
            event.stopPropagation()
            apply('severity', [])
          }}>Clear</button
        >{/if}
    {/snippet}
    {@render queryHint()}
    {#if filters.severity.custom && filters.editable}<p
        class="log-filters__hint"
      >
        Edit this severity condition in search.
      </p>{/if}
    {#each LOG_SEVERITIES as level (level.label)}
      <label class="log-filters__choice item-state">
        <input
          type="checkbox"
          class="checkbox checkbox-sm"
          aria-label={`Severity ${level.label}`}
          checked={filters.severity.values.includes(level.label)}
          disabled={filters.severity.custom || !editor?.setQuery}
          onchange={event =>
            change('severity', level.label, event.currentTarget.checked)}
        />
        <span>{level.label}</span>
      </label>
    {/each}
  </FieldGroup>
</FilterPopover>

<style lang="postcss">
  @reference "../../app.css";
  .log-filters__choice {
    @apply flex min-h-9 cursor-pointer items-center gap-3 rounded-lg px-2 py-2 text-sm;
  }
  .log-filters__hint {
    @apply px-2 text-sm;
    color: var(--color-subtle);
  }
</style>
