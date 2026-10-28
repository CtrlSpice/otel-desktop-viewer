<script lang="ts">
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { FilterIcon } from '@hugeicons/core-free-icons'
  import {
    getStaticFieldsForSearch,
    sameFieldDefinition,
    type ColumnVisibility,
  } from '@/constants/fields'
  import { getDynamicAttributes } from '@/services/search-field-service'
  import {
    fieldIdentity,
    usesFieldToken,
    type FieldDefinition,
    type SearchSignal,
  } from '@/search/model'
  import {
    getTimeContext,
    selectionToQueryRangeMs,
  } from '@/contexts/time-context.svelte'

  type Props = {
    signal: SearchSignal
    selectedFields: FieldDefinition[]
    onToggleField: (field: FieldDefinition) => void
    /** Optional text after the filter icon (e.g. “Columns”). */
    label?: string
    /** Column visibility metadata -- renders pinned fields as locked. */
    columnVisibility?: ColumnVisibility[]
  }

  let {
    signal,
    selectedFields,
    onToggleField,
    label,
    columnVisibility,
  }: Props = $props()

  function getColumnCategory(
    fieldName: string
  ): ColumnVisibility['category'] | null {
    if (!columnVisibility) return null
    return columnVisibility.find(c => c.fieldID === fieldName)?.category ?? null
  }

  let popoverOpen = $state(false)
  let buttonEl = $state<HTMLButtonElement | null>(null)
  let popoverEl = $state<HTMLDivElement | null>(null)

  const timeContext = getTimeContext()

  let availableFields = $state<FieldDefinition[]>([])

  $effect(() => {
    const base = [...getStaticFieldsForSearch(signal)]
    availableFields = base
    let cancelled = false
    const t = window.setTimeout(async () => {
      try {
        void selectionToQueryRangeMs(timeContext.selection, Date.now())
        const dynamicAttrs = await getDynamicAttributes(signal)
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

  let filterableFields = $derived(availableFields)

  function isSelected(field: FieldDefinition & { name: string }): boolean {
    return selectedFields.some(f => sameFieldDefinition(f, field))
  }

  function togglePopover() {
    popoverOpen = !popoverOpen
  }

  function handleClickOutside(event: MouseEvent) {
    if (!popoverOpen) return
    const target = event.target
    if (!(target instanceof Node)) return
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
    class="toolbar-filter-trigger toolbar-filter-trigger--field tooltip tooltip-bottom"
    class:toolbar-filter-trigger--compact={!label}
    class:toolbar-filter-trigger--active={popoverOpen}
    onclick={togglePopover}
    aria-label={label ? `${label}: filter columns` : 'Filter columns'}
    data-tip="Filter columns"
    aria-expanded={popoverOpen}
  >
    <span class="toolbar-filter-trigger__icon" aria-hidden="true">
      <HugeiconsIcon
        icon={FilterIcon}
        size="1em"
        strokeWidth={1.5}
        class="h-4 w-4 shrink-0"
        aria-hidden="true"
      />
    </span>
    {#if label}
      <span class="toolbar-filter-trigger__label">{label}</span>
    {/if}
    {#if activeCount > 0}
      <span class="toolbar-filter-trigger__badge"
        >{activeCount} {activeCount === 1 ? 'field' : 'fields'}</span
      >
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
    <div bind:this={popoverEl} class="popover-surface field-filter__popover">
      <div class="field-filter__popover-header">
        <span class="text-sm font-normal text-subtle">Columns</span>
      </div>
      <div class="field-filter__popover-list">
        {#each filterableFields as field (fieldIdentity(field))}
          {@const checked = isSelected(field)}
          {@const category = getColumnCategory(field.name)}
          {@const pinned = category === 'pinned'}
          <button
            type="button"
            class="field-filter__option item-state item-state--menu"
            aria-pressed={checked}
            class:field-filter__option--pinned={pinned}
            disabled={pinned}
            onclick={() => onToggleField(field)}
          >
            {#if pinned}
              <span class="field-filter__lock" aria-label="Pinned column">
                <svg
                  class="h-3 w-3"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2"
                  stroke-linecap="round"
                  stroke-linejoin="round"
                >
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
            {#if usesFieldToken(field)}
              <span class="text-subtle" aria-hidden="true">·</span>
              <span class="text-secondary">{field.owner}</span>
            {/if}
          </button>
        {/each}
      </div>
    </div>
  {/if}
</div>

<style lang="postcss">
  @reference "../../../app.css";

  .field-filter {
    @apply relative;
  }

  .field-filter__popover {
    @apply absolute right-0 top-full z-50 mt-1;
    @apply min-w-[14rem] max-w-[20rem];
  }

  .field-filter__popover-header {
    @apply px-3 py-2;
  }

  .field-filter__popover-list {
    @apply max-h-64 overflow-y-auto py-1;
    scrollbar-width: thin;
  }

  .field-filter__option {
    @apply flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm;
  }

  .field-filter__option--pinned {
    @apply opacity-50 cursor-default;
  }

  .field-filter__lock {
    @apply flex h-4 w-4 shrink-0 items-center justify-center text-base-content/40;
  }

  .field-filter__checkbox {
    @apply flex h-4 w-4 shrink-0 items-center justify-center rounded;
    border: 1px solid var(--color-highlight-high);
    background-color: var(--color-input);
    @apply transition-colors duration-150;
  }

  .field-filter__checkbox--checked {
    @apply border-primary bg-primary text-primary-content;
  }

  .field-filter__field-name {
    @apply min-w-0 truncate font-sans text-sm text-(--color-subtle);
  }
</style>
