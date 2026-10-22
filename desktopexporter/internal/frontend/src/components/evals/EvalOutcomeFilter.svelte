<script lang="ts">
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { FilterIcon, Tick02Icon } from '@hugeicons/core-free-icons'
  import FilterPopover from '@/components/shared/Toolbar/FilterPopover.svelte'
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
  compact
  showCount
  count={selected.length}
  clearable={draft.length > 0}
  onOpenChange={open => {
    if (open) draft = [...selected]
  }}
  onClear={() => (draft = [])}
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
      disabled={!changed}
      onclick={() => {
        onchange(draft)
        close()
      }}>Apply</button
    >
  {/snippet}
  <div class="outcome-options">
    {#each options as option (option.key)}
      {@const active = draft.includes(option.key)}
      <button
        type="button"
        class="item-state flex min-h-8 min-w-0 cursor-pointer items-center gap-2 px-3 py-1 text-left text-sm font-normal wrap-anywhere text-base-content"
        aria-pressed={active}
        title={`${option.label}: ${option.count} ${option.count === 1 ? 'experiment' : 'experiments'}`}
        onclick={() =>
          (draft = active
            ? draft.filter(key => key !== option.key)
            : [...draft, option.key])}
      >
        <EvalStatus label={option.label} tone={option.tone} />
        <span class="ml-auto shrink-0 text-rp-subtle tabular-nums"
          >[{option.count}]</span
        >
        <span
          class="flex size-4 shrink-0 items-center justify-center"
          class:invisible={!active}
          aria-hidden="true"
        >
          <HugeiconsIcon icon={Tick02Icon} size={16} strokeWidth={1.5} />
        </span>
      </button>
    {:else}
      <p class="text-sm text-rp-subtle">No outcomes in this source.</p>
    {/each}
  </div>
</FilterPopover>

<style lang="postcss">
  @reference "../../app.css";
  .outcome-options {
    @apply grid gap-2;
    grid-template-columns: repeat(auto-fit, minmax(min(100%, 10.5rem), 1fr));
  }
</style>
