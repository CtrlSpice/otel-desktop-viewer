<script lang="ts">
  import {
    getTimeContext,
    TIME_RANGE_PRESETS,
  } from '@/contexts/time-context.svelte'

  const ctx = getTimeContext()

  function applyPreset(preset: (typeof TIME_RANGE_PRESETS)[number]) {
    ctx.setSelection(
      preset.durationMs === undefined
        ? { type: 'all' }
        : { type: 'preset', durationMs: preset.durationMs }
    )
  }
</script>

<div
  class="preset-time-ranges tabs tabs-border flex min-w-0 w-full flex-nowrap overflow-x-auto"
>
  {#each TIME_RANGE_PRESETS as preset (preset.label)}
    {@const selected =
      preset.durationMs === undefined
        ? ctx.selection.type === 'all'
        : ctx.selection.type === 'preset' &&
          ctx.selection.durationMs === preset.durationMs}
    <button
      type="button"
      class="tab flex-auto justify-center whitespace-nowrap px-3 text-sm tabular-nums"
      class:tab-active={selected}
      aria-pressed={selected}
      aria-label={preset.label === 'All' ? 'All time' : `Last ${preset.label}`}
      onclick={() => applyPreset(preset)}
    >
      {preset.shortLabel}
    </button>
  {/each}
</div>
