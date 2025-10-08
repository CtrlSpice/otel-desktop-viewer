<script lang="ts">
  import { getTimeContext } from '@/contexts/time-context.svelte';
  import '@/components/PageHeader/PageHeader.css';

  // Get time context
  let ctx = getTimeContext();
  if (!ctx) {
    throw new Error(
      'Time context not found. Make sure createTimeContext() is called at the root level.'
    );
  }

  // ===== PRESET TIME RANGES =====
  const PRESETS = [
    { label: 'Last 5 minutes', duration: 300000 }, // 5 * 60 * 1000
    { label: 'Last 15 minutes', duration: 900000 }, // 15 * 60 * 1000
    { label: 'Last 30 minutes', duration: 1800000 }, // 30 * 60 * 1000
    { label: 'Last hour', duration: 3600000 }, // 60 * 60 * 1000
    { label: 'Last 3 hours', duration: 10800000 }, // 3 * 60 * 60 * 1000
    { label: 'Last 6 hours', duration: 21600000 }, // 6 * 60 * 60 * 1000
    { label: 'Last day', duration: 86400000 }, // 24 * 60 * 60 * 1000
    { label: 'Last 3 days', duration: 259200000 }, // 3 * 24 * 60 * 60 * 1000
    { label: 'Last week', duration: 604800000 }, // 7 * 24 * 60 * 60 * 1000
    { label: 'Show all', duration: undefined },
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

<div class="space-y-0">
  {#each PRESETS as preset, index}
    <button
      class="list-button {ctx.selection.type === 'preset' &&
      ctx.selection.presetIndex === index
        ? 'selection-indicator--active'
        : ''}"
      onclick={() => applyPreset(index)}
    >
      <span>{preset.label}</span>
    </button>
  {/each}
</div>
