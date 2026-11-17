<script lang="ts">
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { FilterIcon } from '@hugeicons/core-free-icons'
  import FilterPopover from '@/components/shared/Toolbar/FilterPopover.svelte'
  import FilterOption from '@/components/shared/Toolbar/FilterOption.svelte'
  import EvalStatus from './EvalStatus.svelte'
  import type { ExperimentOutcomeOption } from './experiment-outcome-filter'

  let {
    options,
    selected,
    onchange,
  }: {
    options: ExperimentOutcomeOption[]
    selected: string[]
    onchange: (keys: string[]) => void
  } = $props()
  let draft = $state<string[]>([])
  const changed = $derived(
    draft.length !== selected.length ||
      draft.some(key => !selected.includes(key))
  )
</script>

<FilterPopover
  label="Outcomes"
  popoverClass="filter-popover--choices"
  compact
  showCount
  count={selected.length}
  onOpenChange={open => {
    if (open) draft = [...selected]
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
      class="btn btn-ghost btn-sm"
      aria-label="Clear outcomes filter"
      disabled={draft.length === 0}
      onclick={() => (draft = [])}>Clear</button
    >
    <button
      type="button"
      class="btn btn-ghost btn-sm text-sm font-normal text-primary"
      disabled={!changed}
      onclick={() => {
        onchange(draft)
        close()
      }}>Apply</button
    >
  {/snippet}
  <div class="filter-options">
    {#each options as option (option.key)}
      {@const active = draft.includes(option.key)}
      <FilterOption
        selected={active}
        title={`${option.label}: ${option.count} ${option.count === 1 ? 'experiment' : 'experiments'}`}
        onclick={() =>
          (draft = active
            ? draft.filter(key => key !== option.key)
            : [...draft, option.key])}
      >
        <EvalStatus label={option.label} tone={option.tone} />
        <span class="ml-auto shrink-0 text-subtle tabular-nums"
          >[{option.count}]</span
        >
      </FilterOption>
    {:else}
      <p class="text-sm text-subtle">No outcomes in this source.</p>
    {/each}
  </div>
</FilterPopover>
