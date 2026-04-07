<script lang="ts">
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
<div>
  <PresetTimeRanges />

  <div class="p-2">
    <CustomTimeRange />
  </div>

  <div class="border-t border-base-300"></div>

  <div class="p-2">
    <div class="min-w-0">
      <div
        class="section-header mb-1 text-xs font-semibold uppercase tracking-wide text-base-content/70"
      >
        Timezone
      </div>
      <button
        type="button"
        class="timezone-toggle"
        onclick={() => {
          ctx.setTimezone(ctx.timezone === 'UTC' ? 'local' : 'UTC')
        }}
      >
        <div class="flex min-w-0 items-center gap-1.5 font-semibold leading-snug">
          <svg class="h-3.5 w-3.5 shrink-0" viewBox="0 0 24 24">
            <path
              stroke-linejoin="round"
              stroke-width="1.5"
              d="M12 22C6.477 22 2 17.523 2 12a9.97 9.97 0 0 1 2.99-7.132M12 22c-.963-.714-.81-1.544-.326-2.375c.743-1.278.743-1.278.743-2.98c0-1.704 1.012-2.502 4.583-1.788c1.605.321 2.774-1.896 4.857-1.164M12 22c4.946 0 9.053-3.59 9.857-8.307m0 0Q22 12.867 22 12c0-4.881-3.498-8.946-8.123-9.824m0 0c.51.94.305 2.06-.774 2.487c-1.76.697-.5 1.98-2 2.773c-1 .528-2.499.396-3.998-1.189c-.79-.834-1.265-1.29-2.115-1.379m8.887-2.692A10 10 0 0 0 12 2a9.97 9.97 0 0 0-7.01 2.868"
            />
          </svg>
          <span class="min-w-0 truncate">
            {ctx.timezone === 'UTC'
              ? 'Coordinated Universal Time UTC'
              : getLocalTimezoneName()}
          </span>
        </div>
        <div class="flex shrink-0 items-center gap-1 text-base-content/60">
          <span>{ctx.timezone === 'UTC' ? 'UTC+0' : 'Local'}</span>
          <svg class="h-3.5 w-3.5" viewBox="0 0 24 24">
            <path d="M8 9l4-4 4 4m0 6l-4 4-4-4"></path>
          </svg>
        </div>
      </button>
    </div>
  </div>

  <div class="border-t border-base-300"></div>

  <div class="p-2">
    <RecentTimeRanges />
  </div>
</div>

<style lang="postcss">
  @reference "../../../app.css";

  .timezone-toggle {
    @apply flex w-full cursor-pointer items-center justify-between gap-2 rounded-md border-none bg-transparent px-1.5 py-1 text-left text-xs transition-colors;
    @apply text-base-content/90 hover:bg-base-200/90 focus-visible:bg-base-200/90;
    @apply focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/30 focus-visible:ring-offset-0;
  }
</style>
