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
    { label: '5m', duration: 300000 }, // 5 * 60 * 1000
    { label: '15m', duration: 900000 }, // 15 * 60 * 1000
    { label: '30m', duration: 1800000 }, // 30 * 60 * 1000
    { label: '1h', duration: 3600000 }, // 60 * 60 * 1000
    { label: '3h', duration: 10800000 }, // 3 * 60 * 60 * 1000
    { label: '6h', duration: 21600000 }, // 6 * 60 * 60 * 1000
    { label: '24h', duration: 86400000 }, // 24 * 60 * 60 * 1000
    { label: '3d', duration: 259200000 }, // 3 * 24 * 60 * 60 * 1000
    { label: '7d', duration: 604800000 }, // 7 * 24 * 60 * 60 * 1000
    { label: 'All', duration: undefined },
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

<div class="flex items-center gap-2 px-2 py-2 bg-base-200">
  {#each PRESETS as preset, index}
    <button
      class="w-10 h-10 rounded-full text-sm font-medium transition-colors flex items-center justify-center {ctx.selection.type === 'preset' &&
      ctx.selection.presetIndex === index
        ? 'bg-primary text-primary-content'
        : 'text-base-content hover:bg-base-300'}"
      onclick={() => applyPreset(index)}
    >
      {preset.label}
    </button>
  {/each}
</div>
