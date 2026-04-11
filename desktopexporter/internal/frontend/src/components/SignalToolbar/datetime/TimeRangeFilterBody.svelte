<script lang="ts">
  import { GlobalIcon } from '@/icons'
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import { getLocalTimezoneName } from '@/utils/time'
  import PresetTimeRanges from './PresetTimeRanges.svelte'
  import CustomTimeRange from './CustomTimeRange.svelte'
  import RecentTimeRanges from './RecentTimeRanges.svelte'

  let ctx = getTimeContext()
  if (!ctx) {
    throw new Error(
      'Time context not found. Ensure createTimeContext() runs at app root.'
    )
  }
</script>

<!-- Shared body: presets, custom range, timezone, recents (toolbar popover + DateTimeFilter dialog). -->
<div class="text-sm">
  <PresetTimeRanges />

  <div class="min-w-0 w-full">
    <CustomTimeRange />
  </div>

  <div class="border-t border-base-300"></div>

  <div class="min-w-0 w-full">
    <button
      type="button"
      class="timezone-toggle"
      onclick={() => {
        ctx.setTimezone(ctx.timezone === 'UTC' ? 'local' : 'UTC')
      }}
    >
      <div class="flex min-w-0 items-center gap-1.5 leading-snug">
        <GlobalIcon class="h-3.5 w-3.5 text-base-content/55" />
        <span class="table-header-typography min-w-0 truncate">
          {ctx.timezone === 'UTC'
            ? 'Coordinated Universal Time UTC'
            : getLocalTimezoneName()}
        </span>
      </div>
      <div class="flex shrink-0 items-center gap-1 text-sm text-base-content">
        <span>{ctx.timezone === 'UTC' ? 'UTC+0' : 'Local'}</span>
        <svg class="h-3.5 w-3.5" viewBox="0 0 24 24" aria-hidden="true">
          <path d="M8 9l4-4 4 4m0 6l-4 4-4-4"></path>
        </svg>
      </div>
    </button>
  </div>

  <div class="border-t border-base-300"></div>

  <div class="min-w-0 w-full">
    <RecentTimeRanges />
  </div>
</div>

<style lang="postcss">
  @reference "../../../app.css";

  .timezone-toggle {
    box-sizing: border-box;
    height: var(--table-header-h);
    min-height: var(--table-header-h);
    @apply flex w-full cursor-pointer items-center justify-between gap-2 rounded-none border-none bg-transparent px-3 py-1 text-left text-sm transition-colors;
    @apply text-base-content/90 hover:bg-base-200/90 focus-visible:bg-base-200/90;
    @apply focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/30 focus-visible:ring-offset-0;
  }
</style>
