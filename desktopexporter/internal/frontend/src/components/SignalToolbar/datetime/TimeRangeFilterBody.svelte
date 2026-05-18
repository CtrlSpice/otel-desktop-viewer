<script lang="ts">
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import { getLocalTimezoneName, formatTimezoneLabel } from '@/utils/time'
  import { GlobalIcon, CustomizeIcon } from '@/icons'
  import FieldGroup from '@/components/FieldGroup.svelte'
  import CustomTimeRange from './CustomTimeRange.svelte'
  import RecentTimeRanges from './RecentTimeRanges.svelte'

  let ctx = getTimeContext()
  if (!ctx) {
    throw new Error(
      'Time context not found. Ensure createTimeContext() runs at app root.'
    )
  }
</script>

<!-- Shared body: custom range, timezone group, recents group. -->
<div class="flex min-w-0 flex-col text-sm">
  <FieldGroup label="Custom Range">
    {#snippet heading()}
      <CustomizeIcon class="h-3.5 w-3.5 shrink-0 text-base-content/55" />
      <span>Custom Range</span>
    {/snippet}
    <CustomTimeRange />
  </FieldGroup>

  <FieldGroup label="Timezone" open={false}>
    {#snippet heading()}
      <GlobalIcon class="h-3.5 w-3.5 shrink-0 text-base-content/55" />
      <span>Timezone</span>
      <span class="badge-count">{formatTimezoneLabel(ctx.timezone)}</span>
    {/snippet}
    <button
      type="button"
      class="tz-option"
      class:tz-option--active={ctx.timezone === 'local'}
      onclick={() => ctx.setTimezone('local')}
    >
      <span class="min-w-0 flex-1 truncate">{getLocalTimezoneName()}</span>
      <span class="tz-badge badge-count">{formatTimezoneLabel('local')}</span>
    </button>
    <button
      type="button"
      class="tz-option"
      class:tz-option--active={ctx.timezone === 'UTC'}
      onclick={() => ctx.setTimezone('UTC')}
    >
      <span class="min-w-0 flex-1 truncate">Coordinated Universal Time</span>
      <span class="tz-badge badge-count">UTC</span>
    </button>
  </FieldGroup>

  <RecentTimeRanges last />
</div>

<style lang="postcss">
  @reference "../../../app.css";

  .tz-option {
    box-sizing: border-box;
    height: var(--table-row-h);
    min-height: var(--table-row-h);
    @apply flex w-full cursor-pointer items-center gap-2 rounded-none border-none bg-transparent px-0 py-0 text-left text-sm transition-colors;
    @apply text-base-content/90 hover:bg-base-300/40;
    @apply focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/30 focus-visible:ring-offset-0;
  }

  .tz-option--active {
    @apply text-primary;
  }

  .tz-badge {
    @apply ml-auto shrink-0;
  }
</style>
