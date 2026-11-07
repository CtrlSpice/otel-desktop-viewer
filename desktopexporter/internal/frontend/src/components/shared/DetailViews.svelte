<script lang="ts" generics="T">
  import type { Snippet } from 'svelte'
  import RawJsonInspector, {
    type RawJsonScope,
  } from './RawJsonInspector.svelte'
  import DetailViewTabs, { type DetailViewMode } from './DetailViewTabs.svelte'
  import type { RawJsonStatusData } from './RawJsonStatus.svelte'

  const localID = $props.id()
  let {
    scopes,
    wholeURL,
    reveal,
    children,
    id = localID,
    showTabs = true,
    view = $bindable<DetailViewMode>('fields'),
    rawStatus = $bindable<RawJsonStatusData>(),
    showRawStatus = true,
    refreshKey,
    record,
    displayedRecord = $bindable<T | undefined>(),
  }: {
    scopes: RawJsonScope[]
    wholeURL: string
    reveal?: unknown
    children: Snippet
    id?: string
    showTabs?: boolean
    view?: DetailViewMode
    rawStatus?: RawJsonStatusData
    showRawStatus?: boolean
    refreshKey?: unknown
    record?: T
    displayedRecord?: T
  } = $props()
  let mountedRaw = $state(false)
  $effect(() => {
    if (reveal) view = 'fields'
  })
  $effect(() => {
    if (view === 'raw') mountedRaw = true
  })
</script>

{#if showTabs}<div class="px-2"><DetailViewTabs {id} bind:view /></div>{/if}
<div
  id={`${id}-fields`}
  role="tabpanel"
  aria-labelledby={`${id}-fields-tab`}
  hidden={view !== 'fields'}
>
  {@render children()}
</div>
<div
  id={`${id}-raw`}
  role="tabpanel"
  aria-labelledby={`${id}-raw-tab`}
  hidden={view !== 'raw'}
>
  {#if mountedRaw}<RawJsonInspector
      {scopes}
      {wholeURL}
      {refreshKey}
      {record}
      bind:displayedRecord
      bind:status={rawStatus}
      showStatus={showRawStatus}
      active={view === 'raw'}
    />{/if}
</div>
