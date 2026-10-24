<script lang="ts">
  import {
    getTimeContext,
    TIME_RANGE_PRESETS,
  } from '@/contexts/time-context.svelte'

  const ctx = getTimeContext()
  const selectedIndex = $derived(
    TIME_RANGE_PRESETS.findIndex(preset =>
      preset.durationMs === undefined
        ? ctx.selection.type === 'all'
        : ctx.selection.type === 'preset' &&
          ctx.selection.durationMs === preset.durationMs
    )
  )

  function applyPreset(preset: (typeof TIME_RANGE_PRESETS)[number]) {
    ctx.setSelection(
      preset.durationMs === undefined
        ? { type: 'all' }
        : { type: 'preset', durationMs: preset.durationMs }
    )
  }
</script>

<div
  class="preset-time-ranges header-tabs min-w-0 w-full"
  style:--header-tab-count={TIME_RANGE_PRESETS.length}
  style:--header-active-index={selectedIndex}
  style:--header-control-padding="0.25rem"
>
  {#each TIME_RANGE_PRESETS as preset, index (preset.label)}
    <button
      type="button"
      class="btn btn-xs header-tab min-w-0 w-full tabular-nums"
      aria-pressed={selectedIndex === index}
      aria-label={preset.label === 'All' ? 'All time' : `Last ${preset.label}`}
      onclick={() => applyPreset(preset)}
    >
      {preset.shortLabel}
    </button>
  {/each}
</div>
