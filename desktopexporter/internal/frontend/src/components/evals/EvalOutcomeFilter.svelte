<script lang="ts">
  import FilterPopover from '@/components/shared/Toolbar/FilterPopover.svelte'
  import EvalStatus from './EvalStatus.svelte'
  import type { RunOutcomeOption } from './run-outcome-filter'

  let {
    options,
    selected,
    onchange,
  }: {
    options: RunOutcomeOption[]
    selected: string[]
    onchange: (keys: string[]) => void
  } = $props()
</script>

<FilterPopover
  label="Outcomes"
  count={selected.length}
  onClear={() => onchange([])}
>
  <p class="mb-2 text-sm text-rp-subtle">
    Runs containing any selected outcome. Counts are runs.
  </p>
  <div class="flex flex-col gap-1">
    {#each options as option (option.key)}
      {@const active = selected.includes(option.key)}
      <button
        type="button"
        class="item-state flex min-h-8 min-w-0 cursor-pointer items-center gap-2 px-3 py-1 text-left text-sm font-normal wrap-anywhere text-base-content"
        aria-pressed={active}
        title={`${option.label}: ${option.count} ${option.count === 1 ? 'run' : 'runs'}`}
        onclick={() =>
          onchange(
            active
              ? selected.filter(key => key !== option.key)
              : [...selected, option.key]
          )}
      >
        <EvalStatus label={option.label} tone={option.tone} />
        <span class="shrink-0 text-rp-subtle tabular-nums"
          >[{option.count}]</span
        >
        <span class="ml-auto" class:invisible={!active} aria-hidden="true"
          >✓</span
        >
      </button>
    {:else}
      <p class="text-sm text-rp-subtle">No outcomes in this source.</p>
    {/each}
  </div>
</FilterPopover>
