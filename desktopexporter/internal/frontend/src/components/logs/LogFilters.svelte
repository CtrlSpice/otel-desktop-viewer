<script lang="ts">
  import type { FieldDefinition } from '@/search/model'
  import type { SearchEditorAPI } from '@/components/shared/Search/search-editor-api'
  import FilterPopover from '@/components/shared/Toolbar/FilterPopover.svelte'
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
  let draftQuery = $state('')
  let filters = $derived(readLogFilters(draftQuery, fields))
  let appliedFilters = $derived(readLogFilters(query, fields))
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

{#snippet queryHint()}
  {#if !filters.editable}<p class="log-filters__hint mb-2">
      Use a complete field expression in search to combine it with filters.
    </p>{/if}
{/snippet}

<FilterPopover
  label="Filters"
  compact
  count={appliedFilters.service.values.length +
    appliedFilters.severity.values.length}
  custom={appliedFilters.editable &&
    (appliedFilters.service.custom || appliedFilters.severity.custom)}
  clearable={filters.service.values.length + filters.severity.values.length > 0}
  onOpenChange={open => {
    if (open) {
      draftQuery = query
      serviceSearch = ''
    }
  }}
  onClear={clearFilters}
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
      class="btn btn-primary btn-sm"
      disabled={!editor?.setQuery || draftQuery === query || !filters.editable}
      onclick={() => {
        editor?.setQuery?.(draftQuery)
        close()
      }}>Apply</button
    >
  {/snippet}
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
    {#if filters.service.values.length}
      <div
        class="log-filters__chips mb-2"
        role="group"
        aria-label="Selected services"
      >
        {#each filters.service.values as service (service)}
          <button
            type="button"
            class="log-filters__service item-state"
            aria-label={`Remove service ${service}`}
            onclick={() => change('service', service, false)}
          >
            <span
              class="log-filters__dot"
              style:background-color={traceServiceColor(
                service,
                themeSignal.value
              )}
              aria-hidden="true"
            ></span>
            <span class="min-w-0 truncate" title={service}>{service}</span>
            <span aria-hidden="true">×</span>
          </button>
        {/each}
      </div>
    {/if}
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
    <div class="log-filters__chips">
      {#each visibleServices as service (service)}
        <button
          type="button"
          class="log-filters__service item-state"
          aria-label={`Service ${service}`}
          aria-pressed={filters.service.values.includes(service)}
          disabled={filters.service.custom || !editor?.setQuery}
          onclick={() =>
            change(
              'service',
              service,
              !filters.service.values.includes(service)
            )}
        >
          <span
            class="log-filters__dot"
            style:background-color={traceServiceColor(
              service,
              themeSignal.value
            )}
            aria-hidden="true"
          ></span>
          <span class="min-w-0 truncate" title={service}>{service}</span>
          {#if filters.service.values.includes(service)}<span aria-hidden="true"
              >✓</span
            >{/if}
        </button>
      {:else}<p class="log-filters__hint">
          {serviceSearch
            ? 'No matching services.'
            : 'No named services in the loaded logs.'}
        </p>{/each}
    </div>
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
    <div class="log-filters__chips">
      {#each LOG_SEVERITIES as level (level.label)}
        <button
          type="button"
          class="log-filters__severity item-state"
          aria-label={`Severity ${level.label}`}
          aria-pressed={filters.severity.values.includes(level.label)}
          disabled={filters.severity.custom || !editor?.setQuery}
          onclick={() =>
            change(
              'severity',
              level.label,
              !filters.severity.values.includes(level.label)
            )}
        >
          <span
            class="log-filters__dot"
            style:background-color={severityColor(level.min)}
            aria-hidden="true"
          ></span>
          <span>{level.label}</span>
          <span
            class="log-filters__check"
            class:log-filters__check--selected={filters.severity.values.includes(
              level.label
            )}
            aria-hidden="true">✓</span
          >
        </button>
      {/each}
    </div>
  </FieldGroup>
</FilterPopover>

<style lang="postcss">
  @reference "../../app.css";
  .log-filters__chips {
    @apply flex min-w-0 flex-wrap gap-2 px-2 py-1;
  }
  .log-filters__service,
  .log-filters__severity {
    @apply inline-flex min-h-8 max-w-full cursor-pointer items-center gap-2 border-0 px-3 py-1 text-sm font-normal text-base-content;
  }
  .log-filters__dot {
    @apply size-2 shrink-0 rounded-full;
  }
  .log-filters__check {
    opacity: 0;
  }
  .log-filters__check--selected {
    opacity: 1;
  }
  .log-filters__service:disabled,
  .log-filters__severity:disabled {
    @apply cursor-default opacity-50;
  }
  .log-filters__hint {
    @apply px-2 text-sm;
    color: var(--color-subtle);
  }
</style>
