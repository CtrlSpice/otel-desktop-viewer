<script lang="ts">
  import { getTimeContext } from '@/contexts/time-context.svelte'

  let ctx = getTimeContext()
  if (!ctx) {
    throw new Error(
      'Time context not found. Make sure createTimeContext() is called at the root level.'
    )
  }

  const PRESETS = [
    { label: 'All', duration: undefined },
    { label: '5m', duration: 300000 },
    { label: '15m', duration: 900000 },
    { label: '30m', duration: 1800000 },
    { label: '1h', duration: 3600000 },
    { label: '6h', duration: 21600000 },
    { label: '24h', duration: 86400000 },
    { label: '7d', duration: 604800000 },
  ] as const

  function applyPreset(index: number) {
    const preset = PRESETS[index]
    if (!preset) return
    ctx.setSelection(
      preset.duration === undefined
        ? { type: 'all' }
        : { type: 'preset', presetIndex: index, durationMs: preset.duration }
    )
  }
</script>

<div class="preset-time-ranges">
  {#each PRESETS as preset, index (preset.label)}
    {@const selected =
      index === 0
        ? ctx.selection.type === 'all'
        : ctx.selection.type === 'preset' &&
          ctx.selection.presetIndex === index}
    <button
      type="button"
      class="chrome-btn {selected
        ? 'chrome-btn--active'
        : 'chrome-btn--inactive'}"
      aria-pressed={selected}
      aria-label={preset.label === 'All' ? 'All time' : `Last ${preset.label}`}
      onclick={() => applyPreset(index)}
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
