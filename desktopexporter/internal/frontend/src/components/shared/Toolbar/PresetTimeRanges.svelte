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

<div class="preset-time-ranges tabs tabs-border">
  {#each TIME_RANGE_PRESETS as preset (preset.label)}
    {@const selected =
      preset.durationMs === undefined
        ? ctx.selection.type === 'all'
        : ctx.selection.type === 'preset' &&
          ctx.selection.durationMs === preset.durationMs}
    <button
      type="button"
      class="tab min-w-0 px-3 text-sm first:justify-start last:justify-end before:left-0 before:w-full before:text-primary {selected
        ? 'before:h-px before:border-t'
        : ''}"
      class:tab-active={selected}
      aria-pressed={selected}
      aria-label={preset.label === 'All' ? 'All time' : `Last ${preset.label}`}
      onclick={() => applyPreset(preset)}
    >
      {preset.label}
    </button>
  {/each}
</div>

<style lang="postcss">
  @reference "../../../app.css";

  .preset-time-ranges {
    @apply grid min-w-0 w-full grid-flow-col auto-cols-fr;
  }
</style>
