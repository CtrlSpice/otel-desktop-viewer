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

<div class="preset-time-ranges">
  {#each TIME_RANGE_PRESETS as preset (preset.label)}
    {@const selected =
      preset.durationMs === undefined
        ? ctx.selection.type === 'all'
        : ctx.selection.type === 'preset' &&
          ctx.selection.durationMs === preset.durationMs}
    <button
      type="button"
      class="chrome-btn {selected
        ? 'chrome-btn--active'
        : 'chrome-btn--inactive'}"
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
    @apply flex min-w-0 w-full flex-nowrap items-center justify-evenly gap-1;
  }

  .preset-time-ranges .chrome-btn--inactive {
    color: var(--color-base-content);
  }

  .preset-time-ranges .chrome-btn--active {
    color: var(--color-primary);
  }
</style>
