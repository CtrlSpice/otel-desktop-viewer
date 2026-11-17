<script module lang="ts">
  export { paneTabID } from './PaneHeader.svelte'
</script>

<script lang="ts">
  import { getAllContexts, onDestroy, untrack } from 'svelte'
  import PaneHeader, { type PaneHeaderProps } from './PaneHeader.svelte'
  import WorkspaceContent from './WorkspaceContent.svelte'
  import { getWorkspaceHeader } from '@/contexts/workspace-header.svelte'

  let props: PaneHeaderProps = $props()
  const header = getWorkspaceHeader()
  const contexts = getAllContexts<Map<unknown, unknown>>()
  function snapshot(): PaneHeaderProps {
    return {
      ...props,
      leading: props.leading ? leading : undefined,
      right: props.right ? right : undefined,
      badge: props.badge ? badge : undefined,
      metaLeft: props.metaLeft ? metaLeft : undefined,
      metaCenter: props.metaCenter ? metaCenter : undefined,
      metaRight: props.metaRight ? metaRight : undefined,
    }
  }
  // Read route props in their owner's update phase. The persistent host must
  // not pull through an outgoing conditional branch while it is being removed.
  let current = $state.raw(untrack(snapshot))
  $effect.pre(() => {
    current = snapshot()
  })
  if (header) onDestroy(header.register(() => current))
</script>

{#snippet leading()}<WorkspaceContent {contexts}
    >{@render props.leading?.()}</WorkspaceContent
  >{/snippet}
{#snippet right()}<WorkspaceContent {contexts}
    >{@render props.right?.()}</WorkspaceContent
  >{/snippet}
{#snippet badge()}<WorkspaceContent {contexts}
    >{@render props.badge?.()}</WorkspaceContent
  >{/snippet}
{#snippet metaLeft()}<WorkspaceContent {contexts}
    >{@render props.metaLeft?.()}</WorkspaceContent
  >{/snippet}
{#snippet metaCenter()}<WorkspaceContent {contexts}
    >{@render props.metaCenter?.()}</WorkspaceContent
  >{/snippet}
{#snippet metaRight()}<WorkspaceContent {contexts}
    >{@render props.metaRight?.()}</WorkspaceContent
  >{/snippet}

{#if !header}<PaneHeader {...props} />{/if}
