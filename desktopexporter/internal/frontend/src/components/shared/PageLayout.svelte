<script lang="ts" generics="T">
  import { getAllContexts, onDestroy } from 'svelte'
  import WorkspaceShell, { type LayoutOptions } from './WorkspaceShell.svelte'
  import WorkspaceContent from './WorkspaceContent.svelte'
  import { getWorkspaceContext } from '@/contexts/workspace-context.svelte'
  import { createWorkspaceHeader } from '@/contexts/workspace-header.svelte'

  const props: LayoutOptions<T> = $props()
  const workspace = getWorkspaceContext()
  const header = createWorkspaceHeader(() => ({
    mode: 'title',
    pill: true,
    title: props.mainTitle ?? props.drawerLabel,
  }))
  const contexts = getAllContexts<Map<unknown, unknown>>()

  // Routes own their content and data requests. App owns the persistent shell.
  // Register synchronously so destination slots resolve before the next paint.
  if (workspace)
    onDestroy(
      workspace.register(() => ({
        ...props,
        header,
        main: mainSlot,
        pageBanner: props.pageBanner ? bannerSlot : undefined,
        detail: props.detail ? detailSlot : undefined,
        itemSnippet: props.itemSnippet ? itemSlot : undefined,
        drawerContent: props.drawerContent ? drawerSlot : undefined,
        drawerSearch: props.drawerSearch ? searchSlot : undefined,
        drawerControls: props.drawerControls ? controlsSlot : undefined,
        drawerChromeToolbar: props.drawerChromeToolbar
          ? toolbarSlot
          : undefined,
        drawerFooter: props.drawerFooter ? drawerFooterSlot : undefined,
        pageFooter: props.pageFooter ? footerSlot : undefined,
      }))
    )
</script>

{#snippet mainSlot()}<WorkspaceContent {contexts}
    >{@render props.main()}</WorkspaceContent
  >{/snippet}
{#snippet bannerSlot()}<WorkspaceContent {contexts}
    >{@render props.pageBanner?.()}</WorkspaceContent
  >{/snippet}
{#snippet detailSlot()}<WorkspaceContent {contexts}
    >{@render props.detail?.()}</WorkspaceContent
  >{/snippet}
{#snippet itemSlot(item: T, selected: boolean)}<WorkspaceContent {contexts}
    >{@render props.itemSnippet?.(item, selected)}</WorkspaceContent
  >{/snippet}
{#snippet drawerSlot()}<WorkspaceContent {contexts}
    >{@render props.drawerContent?.()}</WorkspaceContent
  >{/snippet}
{#snippet searchSlot()}<WorkspaceContent {contexts}
    >{@render props.drawerSearch?.()}</WorkspaceContent
  >{/snippet}
{#snippet controlsSlot()}<WorkspaceContent {contexts}
    >{@render props.drawerControls?.()}</WorkspaceContent
  >{/snippet}
{#snippet toolbarSlot()}<WorkspaceContent {contexts}
    >{@render props.drawerChromeToolbar?.()}</WorkspaceContent
  >{/snippet}
{#snippet drawerFooterSlot()}<WorkspaceContent {contexts}
    >{@render props.drawerFooter?.()}</WorkspaceContent
  >{/snippet}
{#snippet footerSlot()}<WorkspaceContent {contexts}
    >{@render props.pageFooter?.()}</WorkspaceContent
  >{/snippet}

{#if !workspace}
  <WorkspaceShell {...props} {header} />
{/if}
