<script module lang="ts">
  export type DetailViewMode = 'fields' | 'raw'
</script>

<script lang="ts">
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { PropertyNewIcon, ThirdBracketIcon } from '@hugeicons/core-free-icons'

  let {
    id,
    view = $bindable<DetailViewMode>('fields'),
  }: { id: string; view?: DetailViewMode } = $props()
</script>

<div role="tablist" aria-label="Detail view" class="join">
  <button
    id={`${id}-fields-tab`}
    type="button"
    role="tab"
    aria-label="Properties"
    title="Properties"
    class="btn btn-xs join-item chart-header-control"
    class:chart-header-control--active={view === 'fields'}
    aria-selected={view === 'fields'}
    aria-controls={`${id}-fields`}
    onclick={() => (view = 'fields')}
    ><span class="chart-header-control__icon shrink-0"
      ><HugeiconsIcon
        icon={PropertyNewIcon}
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
    aria-label="Raw JSON"
    title="Raw JSON"
    class="btn btn-xs join-item chart-header-control"
    class:chart-header-control--active={view === 'raw'}
    aria-selected={view === 'raw'}
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
    ><span>Raw JSON</span></button
  >
</div>
