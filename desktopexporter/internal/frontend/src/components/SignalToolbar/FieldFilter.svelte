<script lang="ts">
  import { FilterIcon } from '@/icons'
  import {
    getStaticFieldsForSearch,
    getDynamicAttributes,
    sameFieldDefinition,
    type FieldDefinition,
    type ColumnVisibility,
  } from '@/constants/fields'
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import type { TimeContext } from '@/contexts/time-context.svelte'

  type Props = {
    signal: 'traces' | 'metrics' | 'logs'
    selectedFields: FieldDefinition[]
    onToggleField: (field: FieldDefinition) => void
    /** Optional text after the filter icon (e.g. “Columns”). */
    label?: string
    /** Column visibility metadata -- renders pinned fields as locked. */
    columnVisibility?: ColumnVisibility[]
  }

  let { signal, selectedFields, onToggleField, label, columnVisibility }: Props = $props()

  function getColumnCategory(fieldName: string): ColumnVisibility['category'] | null {
    if (!columnVisibility) return null
    return columnVisibility.find(c => c.fieldId === fieldName)?.category ?? null
  }

  let popoverOpen = $state(false)
  let buttonEl = $state<HTMLButtonElement | null>(null)
  let popoverEl = $state<HTMLDivElement | null>(null)

  let timeContext: TimeContext | null = null
  try {
    timeContext = getTimeContext()
  } catch {
    /* no time context available */
  }

  let availableFields = $state<FieldDefinition[]>([])

  $effect(() => {
    const base = [...getStaticFieldsForSearch(signal)]
    availableFields = base
    const tc = timeContext
    if (!tc) return

    let cancelled = false
    const t = window.setTimeout(async () => {
      try {
        const dynamicAttrs = await getDynamicAttributes(
          tc.selection.start,
          tc.selection.end,
          signal
        )
        if (cancelled) return
        availableFields = [...base, ...dynamicAttrs]
      } catch {
        /* keep static fields */
      }
    }, 250)

    return () => {
      cancelled = true
      window.clearTimeout(t)
    }
  })

  /** Only show named fields (not global search scope) */
  let filterableFields = $derived(
    availableFields.filter(
      (f): f is FieldDefinition & { name: string } =>
        f.searchScope !== 'global' && 'name' in f
    )
  )

  function isSelected(field: FieldDefinition & { name: string }): boolean {
    return selectedFields.some(f => sameFieldDefinition(f, field))
  }

  function togglePopover() {
    popoverOpen = !popoverOpen
  }

  function handleClickOutside(event: MouseEvent) {
    if (!popoverOpen) return
    const target = event.target as Node
    if (buttonEl?.contains(target) || popoverEl?.contains(target)) return
    popoverOpen = false
  }

  $effect(() => {
    if (popoverOpen) {
      document.addEventListener('click', handleClickOutside, true)
      return () =>
        document.removeEventListener('click', handleClickOutside, true)
    }
  })

  let activeCount = $derived(selectedFields.length)
</script>

<div class="field-filter">
  <button
    bind:this={buttonEl}
    type="button"
    class="toolbar-filter-trigger toolbar-filter-trigger--field"
    class:toolbar-filter-trigger--compact={!label}
    class:toolbar-filter-trigger--active={popoverOpen}
    onclick={togglePopover}
    aria-label={label ? `${label}: filter columns` : 'Filter columns'}
    title="Filter columns"
    aria-expanded={popoverOpen}
  >
    <span class="toolbar-filter-trigger__icon" aria-hidden="true">
      <FilterIcon />
    </span>
    {#if label}
      <span class="toolbar-filter-trigger__label">{label}</span>
    {/if}
    {#if activeCount > 0}
      <span class="toolbar-filter-trigger__badge">{activeCount}</span>
    {/if}
    {#if label}
      <span class="toolbar-filter-trigger__dropdown-circle" aria-hidden="true">
        <svg
          class="popover-indicator h-3 w-3 shrink-0 {popoverOpen
            ? 'popover-indicator--open'
            : ''}"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="1.5"
          stroke-linecap="round"
          stroke-linejoin="round"
        >
          <path d="M18 9s-4.419 6-6 6s-6-6-6-6" />
        </svg>
      </span>
    {/if}
  </button>

  {#if popoverOpen}
    <div bind:this={popoverEl} class="field-filter__popover">
      <div class="field-filter__popover-header table-header-surface">
        <span
          class="text-xs font-semibold uppercase tracking-wide text-base-content/55"
          >Columns</span
        >
      </div>
      <div class="field-filter__popover-list">
        {#each filterableFields as field (field.name + ':' + field.searchScope)}
          {@const checked = isSelected(field)}
          {@const category = getColumnCategory(field.name)}
          {@const pinned = category === 'pinned'}
          <button
            type="button"
            class="field-filter__option"
            class:field-filter__option--selected={checked}
            class:field-filter__option--pinned={pinned}
            disabled={pinned}
            onclick={() => onToggleField(field)}
          >
            {#if pinned}
              <span class="field-filter__lock" aria-label="Pinned column">
                <svg class="h-3 w-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                  <rect x="3" y="11" width="18" height="11" rx="2" ry="2" />
                  <path d="M7 11V7a5 5 0 0 1 10 0v4" />
                </svg>
              </span>
            {:else}
              <span
                class="field-filter__checkbox"
                class:field-filter__checkbox--checked={checked}
              >
                {#if checked}
                  <svg
                    class="h-3 w-3"
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="3"
                    stroke-linecap="round"
                    stroke-linejoin="round"
                  >
                    <path d="M5 12l5 5L20 7" />
                  </svg>
                {/if}
              </span>
            {/if}
            <span class="field-filter__field-name">{field.name}</span>
            {#if field.searchScope === 'attribute' && 'attributeScope' in field}
              <span class="badge-origin">{field.attributeScope}</span>
            {/if}
          </button>
        {/each}
      </div>
    </div>
  {/if}
</div>

<style lang="postcss">
  @reference "../../app.css";

  .field-filter {
    @apply relative;
  }

  .field-filter__popover {
    @apply absolute right-0 top-full z-50 mt-1;
    @apply min-w-[14rem] max-w-[20rem];
    @apply rounded-lg border border-base-300/70 bg-base-100 shadow-lg;
    @apply overflow-hidden;
  }

  .field-filter__popover-header {
    @apply px-3 py-2 border-b border-base-300/50;
  }

  .field-filter__popover-list {
    @apply max-h-64 overflow-y-auto py-1;
    scrollbar-width: thin;
  }

  .field-filter__option {
    @apply flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm;
    @apply transition-colors duration-150;
    @apply hover:bg-base-200/60;
  }

  .field-filter__option--selected {
    @apply bg-primary/5;
  }

  .field-filter__option--pinned {
    @apply opacity-50 cursor-default;
  }

  .field-filter__lock {
    @apply flex h-4 w-4 shrink-0 items-center justify-center text-base-content/40;
  }

  .field-filter__checkbox {
    @apply flex h-4 w-4 shrink-0 items-center justify-center rounded;
    @apply border border-base-300/70;
    @apply transition-colors duration-150;
  }

  .field-filter__checkbox--checked {
    @apply border-primary/50 bg-primary/15 text-primary;
  }

  .field-filter__field-name {
    @apply min-w-0 truncate font-mono text-xs text-base-content/80;
  }
</style>
