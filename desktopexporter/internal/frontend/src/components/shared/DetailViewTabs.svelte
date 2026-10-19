<script module lang="ts">
  export type DetailViewMode = 'fields' | 'raw'
</script>

<script lang="ts">
  import type { Snippet } from 'svelte'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { Telescope01Icon, ThirdBracketIcon } from '@hugeicons/core-free-icons'
  import { handleTablistKeydown } from './utils/tablist-keyboard'

  let {
    id,
    view = $bindable<DetailViewMode>('fields'),
    action,
  }: { id: string; view?: DetailViewMode; action?: Snippet } = $props()
</script>

<div class="join" role="group" aria-label="Detail controls">
  <div
    role="tablist"
    tabindex="-1"
    aria-label="Detail view"
    class="detail-view-tabs join"
    onkeydown={handleTablistKeydown}
  >
    <button
      id={`${id}-fields-tab`}
      type="button"
      role="tab"
      aria-label="Properties"
      data-tip="Properties"
      class="btn btn-xs join-item chart-header-control tooltip tooltip-left"
      class:chart-header-control--active={view === 'fields'}
      aria-selected={view === 'fields'}
      tabindex={view === 'fields' ? 0 : -1}
      aria-controls={`${id}-fields`}
      onclick={() => (view = 'fields')}
      ><span class="chart-header-control__icon shrink-0"
        ><HugeiconsIcon
          icon={Telescope01Icon}
          size={16}
          strokeWidth={1.5}
          class="shrink-0"
          aria-hidden="true"
        /></span
      ><span>Properties</span></button
    >
    <button
      id={`${id}-raw-tab`}
      type="button"
      role="tab"
      aria-label="JSON"
      data-tip="JSON"
      class="btn btn-xs join-item chart-header-control tooltip tooltip-left"
      class:chart-header-control--active={view === 'raw'}
      aria-selected={view === 'raw'}
      tabindex={view === 'raw' ? 0 : -1}
      aria-controls={`${id}-raw`}
      onclick={() => (view = 'raw')}
      ><span class="chart-header-control__icon shrink-0"
        ><HugeiconsIcon
          icon={ThirdBracketIcon}
          size={16}
          strokeWidth={1.5}
          class="shrink-0"
          aria-hidden="true"
        /></span
      ><span>JSON</span></button
    >
  </div>
  {#if action}{@render action()}{/if}
</div>

<style lang="postcss">
  @reference "../../app.css";

  .detail-view-tabs .chart-header-control {
    @apply h-6 min-h-6 px-2;
  }
</style>
