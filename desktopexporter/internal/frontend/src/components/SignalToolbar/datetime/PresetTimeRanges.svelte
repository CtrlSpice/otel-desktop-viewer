<script lang="ts">
  import { getTimeContext } from '@/contexts/time-context.svelte';

  // Get time context
  let ctx = getTimeContext();
  if (!ctx) {
    throw new Error(
      'Time context not found. Make sure createTimeContext() is called at the root level.'
    );
  }

  // ===== PRESET TIME RANGES =====
  const PRESETS = [
    { label: 'All', duration: undefined },
    { label: '5m', duration: 300000 }, // 5 * 60 * 1000
    { label: '15m', duration: 900000 }, // 15 * 60 * 1000
    { label: '30m', duration: 1800000 }, // 30 * 60 * 1000
    { label: '1h', duration: 3600000 }, // 60 * 60 * 1000
    { label: '6h', duration: 21600000 }, // 6 * 60 * 60 * 1000
    { label: '24h', duration: 86400000 }, // 24 * 60 * 60 * 1000
    { label: '7d', duration: 604800000 }, // 7 * 24 * 60 * 60 * 1000
  ] as const;

  function applyPreset(index: number) {
    let start = 0;
    let now = Date.now();
    let preset = PRESETS[index];

    // Handle "Show all" case
    if (preset.duration !== undefined) {
      start = now - preset.duration;
    }

    ctx.setSelection(start, now, 'preset', index);
  }
</script>

<div
  class="table-header-surface min-w-0 flex-nowrap justify-evenly rounded-t-lg px-2"
>
  {#each PRESETS as preset, index}
    {@const selected =
      ctx.selection.type === 'preset' && ctx.selection.presetIndex === index}
    <button
      type="button"
      class="btn btn-circle btn-sm shrink-0 transition-colors {selected
        ? 'btn-soft btn-primary'
        : 'btn-ghost'}"
      aria-pressed={selected}
      aria-label={preset.label === 'All'
        ? 'All time'
        : `Last ${preset.label}`}
      onclick={() => applyPreset(index)}
    >
      {preset.label}
    </button>
  {/each}
</div>
