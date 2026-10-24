<script module lang="ts">
  export type TableColumnOption = {
    id: string
    label: string
    accessibleLabel?: string
    group: string
    disabled?: boolean
    count?: number
    countTitle?: string
    pinnable?: boolean
  }
</script>

<script lang="ts">
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { LayoutThreeColumnIcon, Pin02Icon } from '@hugeicons/core-free-icons'
  import ListFilter from './Search/ListFilter.svelte'
  import Checkbox from './Checkbox.svelte'
  import { createPopoverID, setupAnchorPopover } from './utils/anchor-popover'
  import { updateColumnSelection } from './utils/column-selection'

  let {
    options,
    selected,
    onchange,
    label = 'Table columns',
    minimumSelected = 0,
    pinned = [],
    onPin,
  }: {
    options: TableColumnOption[]
    selected: string[]
    onchange: (ids: string[]) => void
    label?: string
    minimumSelected?: number
    pinned?: readonly string[]
    onPin?: (id: string) => void
  } = $props()
  const popoverID = createPopoverID('table-columns')
  let trigger = $state<HTMLButtonElement>()
  let popover = $state<HTMLDivElement>()
  let open = $state(false)
  let query = $state('')
  const filtered = $derived(
    options.filter(option =>
      (option.accessibleLabel ?? option.label)
        .toLowerCase()
        .includes(query.trim().toLowerCase())
    )
  )
  const groups = $derived([...new Set(options.map(option => option.group))])
  function toggle(ids: string[], checked: boolean) {
    onchange(
      updateColumnSelection(selected, options, ids, checked, minimumSelected)
    )
  }

  $effect(() => {
    if (!trigger || !popover) return
    const element = popover
    return setupAnchorPopover({
      trigger,
      popover,
      anchor: 'below-end',
      onOpenChange: next => {
        open = next
        if (next)
          element
            .querySelector<HTMLButtonElement>('[data-list-filter-toggle]')
            ?.focus({ preventScroll: true })
      },
    })
  })
</script>

<button
  bind:this={trigger}
  type="button"
  popovertarget={popoverID}
  aria-expanded={open}
  aria-label="Columns"
  data-tip="Select columns"
  class="btn btn-xs chart-header-control tooltip tooltip-left"
>
  <HugeiconsIcon
    icon={LayoutThreeColumnIcon}
    size={16}
    strokeWidth={1.5}
    aria-hidden="true"
  />
</button>
<div
  bind:this={popover}
  id={popoverID}
  popover="auto"
  role="group"
  aria-label={label}
  class="anchor-popover anchor-popover--anchored table-columns-picker"
>
  <ListFilter
    bind:value={query}
    label="Find columns"
    placeholder="Find columns…"
  >
    {#snippet children(filterToggle, filterField)}
      <div class="mb-2 flex min-w-0 items-center gap-2 px-2">
        <span class="min-w-0 flex-1 text-sm text-subtle">{label}</span>
        {@render filterToggle()}
      </div>
      {@render filterField()}
    {/snippet}
  </ListFilter>
  {#each groups as group}
    {@const entries = filtered.filter(option => option.group === group)}
    {@const members = options.filter(option => option.group === group)}
    {@const editable = members.filter(option => !option.disabled)}
    {@const checkedCount = editable.filter(option =>
      selected.includes(option.id)
    ).length}
    {@const allChecked = editable.length
      ? checkedCount === editable.length
      : members.every(option => selected.includes(option.id))}
    {#if entries.length}
      <fieldset class="min-w-0" aria-label={group}>
        <legend class="w-full">
          <label
            class="item-state flex cursor-pointer items-center gap-2 px-2 py-1 text-sm font-normal text-base-content"
          >
            <Checkbox
              label={`All ${group} columns`}
              checked={allChecked}
              indeterminate={checkedCount > 0 && checkedCount < editable.length}
              disabled={!editable.length ||
                (allChecked && selected.length <= minimumSelected)}
              onchange={checked =>
                toggle(
                  members.map(option => option.id),
                  checked
                )}
            />
            <span class="capitalize">{group}</span>
          </label>
        </legend>
        {#each entries as option (option.id)}
          <div
            class="item-state flex items-center gap-2 pl-8 pr-2 py-1.5 text-sm"
          >
            <label
              class="flex min-w-0 flex-1 cursor-pointer items-center gap-2"
            >
              <Checkbox
                label={option.accessibleLabel ?? option.label}
                checked={selected.includes(option.id)}
                disabled={option.disabled ||
                  (selected.includes(option.id) &&
                    selected.length <= minimumSelected)}
                onchange={checked => toggle([option.id], checked)}
              />
              <span class="min-w-0 flex-1 break-words">{option.label}</span>
              {#if option.count !== undefined}<span
                  class="shrink-0 text-sm {option.count
                    ? 'text-base-content'
                    : 'text-muted'}"
                  title={option.countTitle}>{option.count}</span
                >{/if}
            </label>
            {#if onPin && option.pinnable !== false}
              <button
                type="button"
                class="btn btn-ghost btn-circle btn-sm tooltip tooltip-left"
                class:text-primary={pinned.includes(option.id)}
                aria-label={`${pinned.includes(option.id) ? 'Unpin' : 'Pin'} ${option.label} column`}
                aria-pressed={pinned.includes(option.id)}
                data-tip={`${pinned.includes(option.id) ? 'Unpin' : 'Pin'} ${option.label} column`}
                disabled={!selected.includes(option.id)}
                onclick={() => onPin?.(option.id)}
              >
                <HugeiconsIcon
                  icon={Pin02Icon}
                  size={16}
                  strokeWidth={1.5}
                  aria-hidden="true"
                />
              </button>
            {/if}
          </div>
        {/each}
      </fieldset>
    {/if}
  {/each}
  {#if !filtered.length}<p class="p-2 text-sm text-subtle">
      {query ? 'No matching columns.' : 'No columns available.'}
    </p>{/if}
</div>

<style lang="postcss">
  @reference "../../app.css";
  .table-columns-picker {
    @apply overflow-y-auto p-2;
    width: 22rem;
    max-width: calc(100vw - 1.5rem);
    max-height: min(70vh, 32rem);
  }
</style>
