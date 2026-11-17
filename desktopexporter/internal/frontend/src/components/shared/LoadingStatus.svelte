<script lang="ts">
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { Loading03Icon } from '@hugeicons/core-free-icons'
  import {
    createLoadingNotice,
    type LoadingRequestKey,
  } from './utils/loading-notice.svelte'

  let {
    pending,
    label = 'Loading…',
    requestKey,
    compact = false,
    class: className = 'text-sm text-subtle',
  }: {
    pending: boolean
    label?: string
    requestKey?: LoadingRequestKey
    compact?: boolean
    class?: string
  } = $props()

  const notice = createLoadingNotice(
    () => pending,
    () => requestKey
  )
</script>

{#if notice.visible}
  <span
    class="{compact
      ? 'tooltip tooltip-bottom inline-flex h-4 w-4 shrink-0 items-center justify-center'
      : ''} {className}"
    role="status"
    data-tip={compact ? label : undefined}
  >
    {#if compact}
      <HugeiconsIcon
        icon={Loading03Icon}
        size={16}
        strokeWidth={1.5}
        class="motion-safe:animate-spin"
        aria-hidden="true"
      />
      <span class="sr-only">{label}</span>
    {:else}
      {label}
    {/if}
  </span>
{/if}
