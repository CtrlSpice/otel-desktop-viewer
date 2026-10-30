<script lang="ts">
  import {
    createLoadingNotice,
    type LoadingRequestKey,
  } from './utils/loading-notice.svelte'

  let {
    pending,
    label = 'Loading…',
    requestKey,
    class: className = 'text-sm text-subtle',
  }: {
    pending: boolean
    label?: string
    requestKey?: LoadingRequestKey
    class?: string
  } = $props()

  const notice = createLoadingNotice(
    () => pending,
    () => requestKey
  )
</script>

{#if notice.visible}
  <span class={className} role="status">{label}</span>
{/if}
