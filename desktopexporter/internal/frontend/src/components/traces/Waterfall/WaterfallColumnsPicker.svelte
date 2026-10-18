<script lang="ts">
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { LayoutThreeColumnIcon } from '@hugeicons/core-free-icons'
  import {
    createPopoverID,
    setupAnchorPopover,
  } from '@/components/shared/utils/anchor-popover'
  import {
    COLUMN_SOURCES,
    columnID,
    columnLabel,
    type WaterfallColumn,
    type WaterfallColumnOption,
  } from './waterfall-columns'

  let {
    options,
    selected,
    onchange,
  }: {
    options: WaterfallColumnOption[]
    selected: WaterfallColumn[]
    onchange: (columns: WaterfallColumn[]) => void
  } = $props()

  const popoverID = createPopoverID('waterfall-columns')
  let triggerEl = $state<HTMLButtonElement | null>(null)
  let popoverEl = $state<HTMLDivElement | null>(null)
  let searchEl = $state<HTMLInputElement | null>(null)
  let open = $state(false)
  let query = $state('')
  let selectedIDs = $derived(new Set(selected.map(columnID)))
  let filtered = $derived(
    options.filter(option =>
      columnLabel(option).toLowerCase().includes(query.trim().toLowerCase())
    )
  )

  $effect(() => {
    const trigger = triggerEl
    const popover = popoverEl
    if (!trigger || !popover) return
    return setupAnchorPopover({
      trigger,
      popover,
      anchor: 'below-end',
      onOpenChange: next => {
        open = next
        if (next) searchEl?.focus()
      },
    })
  })

  function toggle(option: WaterfallColumnOption, checked: boolean) {
    const column: WaterfallColumn = { source: option.source, key: option.key }
    if (option.field) column.field = true
    onchange(
      checked
        ? [...selected, column]
        : selected.filter(column => columnID(column) !== option.id)
    )
  }
</script>

<button
  bind:this={triggerEl}
  type="button"
  popovertarget={popoverID}
  aria-expanded={open}
  aria-label="Columns"
  data-tip="Columns"
  class="btn btn-xs join-item chart-header-control tooltip tooltip-left"
>
  <HugeiconsIcon
    icon={LayoutThreeColumnIcon}
    size={16}
    strokeWidth={1.5}
    aria-hidden="true"
  />
</button>
<div
  bind:this={popoverEl}
  id={popoverID}
  popover="auto"
  role="group"
  aria-label="Waterfall columns"
  class="anchor-popover anchor-popover--anchored waterfall-columns-picker"
>
  <input
    bind:this={searchEl}
    bind:value={query}
    type="search"
    aria-label="Find columns"
    placeholder="Find columns…"
    class="typed-field typed-field--pill mb-2 w-full"
  />
  {#each COLUMN_SOURCES as source}
    {#each [true, false] as field}
      {@const group = filtered.filter(
        option => option.source === source && Boolean(option.field) === field
      )}
      {#if group.length}
        <fieldset class="min-w-0">
          <legend
            class="px-2 py-1 text-xs font-medium capitalize text-base-content"
            >{source} {field ? 'fields' : 'attributes'}</legend
          >
          {#each group as option (option.id)}
            <label
              class="item-state flex cursor-pointer items-center gap-2 px-2 py-1.5 text-sm"
            >
              <input
                type="checkbox"
                class="checkbox checkbox-xs"
                aria-label={columnLabel(option)}
                checked={selectedIDs.has(option.id)}
                onchange={event => toggle(option, event.currentTarget.checked)}
              />
              <span class="min-w-0 flex-1 break-words"
                >{option.key || '(empty key)'}</span
              >
              {#if option.count !== undefined}<span
                  class="shrink-0 text-xs text-base-content/60"
                  title={option.count
                    ? `${option.count} spans`
                    : 'Not present in this trace'}>{option.count}</span
                >{/if}
            </label>
          {/each}
        </fieldset>
      {/if}
    {/each}
  {/each}
  {#if !filtered.length}<p class="p-2 text-sm text-base-content/70">
      {query ? 'No matching columns.' : 'No columns available.'}
    </p>{/if}
</div>

<style lang="postcss">
  @reference "../../../app.css";
  .waterfall-columns-picker {
    @apply p-2;
    width: 22rem;
    max-width: calc(100vw - 1.5rem);
    max-height: min(70vh, 32rem);
    overflow-y: auto;
  }
</style>
