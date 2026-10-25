<script module lang="ts">
  import type { Snippet } from 'svelte'
  import type { WorkspaceHeaderState } from '@/contexts/workspace-header.svelte'

  export type LayoutOptions<T> = {
    mainTitle?: string
    mainHeaderInert?: boolean
    header?: WorkspaceHeaderState
    // ── Drawer (forwarded to SignalListDrawer) ─────────────────────
    /** Items rendered in the drawer list. Pass `[]` together with
     * `emptyDrawer` for pages with no list yet (Home). On list pages an
     * empty array renders the drawer's empty state instead. */
    items: T[]
    /** Keep the drawer blank on pages with no list yet (Home). */
    emptyDrawer?: boolean
    selectedID: string | null
    drawerID: string
    /** Tooltip label on the collapsed open-drawer button. */
    drawerLabel: string
    itemKey?: (item: T) => string
    /** How each row in the list renders. Optional only because
     * empty-list pages (Home) never call it. */
    itemSnippet?: Snippet<[item: T, selected: boolean]>
    onRefresh?: () => void
    refreshPulse?: boolean
    refreshAsideTip?: string
    loading?: boolean
    drawerChromeToolbar?: Snippet
    drawerSearch?: Snippet
    drawerControls?: Snippet
    drawerFooter?: Snippet
    /** Custom left-pane content, such as Home's welcome, import and services. */
    drawerContent?: Snippet
    /** Omit the list pane and its toggle without changing the saved preference. */
    hideDrawer?: boolean
    revealDrawer?: boolean

    // ── Content slots ──────────────────────────────────────────────
    /** Primary view (waterfall, chart, logs table, home content). */
    main: Snippet
    /** Optional banner spanning List, Main and Details below navigation. */
    pageBanner?: Snippet
    /** Optional inspector pane. When present, content area becomes
     * a resizable horizontal split. */
    detail?: Snippet
    /** Optional page-level footer rendered below the main+detail
     * split, spanning the full content width (everything that
     * isn't the drawer). Use for page-scoped controls that act on
     * the page selection (delete, prev/next, etc.). */
    pageFooter?: Snippet
    /** Signal and Evals main panes use the base surface; Home uses the panel surface. */
    mainSurface?: 'base' | 'panel'
  }
</script>

<script lang="ts" generics="T">
  import ResizablePanels from './ResizablePanels.svelte'
  import SignalListDrawer from './Drawer/SignalListDrawer.svelte'
  import PanelToggle from './PanelToggle.svelte'
  import PaneHeader from './PaneHeader.svelte'
  import {
    detailWidth,
    DETAIL_SPLIT_STORAGE_KEY,
  } from '@/state/detail-width.svelte'
  import { getRouteContext } from '@/contexts/route-context.svelte'
  import { detailPanel } from '@/state/detail-panel.svelte'
  import {
    PANEL_DEFAULT_REM,
    PANEL_MIN_REM,
    remToPx,
  } from '@/state/panel-width'

  let {
    header,
    mainHeaderInert,
    items,
    emptyDrawer,
    selectedID,
    drawerID,
    drawerLabel,
    itemKey,
    itemSnippet,
    onRefresh,
    refreshPulse,
    refreshAsideTip,
    loading,
    drawerChromeToolbar,
    drawerSearch,
    drawerControls,
    drawerFooter,
    drawerContent,
    hideDrawer,
    revealDrawer,
    main,
    pageBanner,
    detail,
    pageFooter,
    mainSurface = 'panel',
  }: LayoutOptions<T> = $props()

  const detailPanelID = $derived(`${drawerID}-detail`)
  const routeContext = getRouteContext()
  let changingRoute = $state(true)
  let keyboardToggle = $state(false)
  $effect(() => {
    if (!keyboardToggle) return
    let frame = requestAnimationFrame(() => {
      frame = requestAnimationFrame(() => (keyboardToggle = false))
    })
    return () => cancelAnimationFrame(frame)
  })
  $effect.pre(() => {
    void routeContext.route.path
    void hideDrawer
    void detail
    changingRoute = true
    let frame = requestAnimationFrame(() => {
      frame = requestAnimationFrame(() => (changingRoute = false))
    })
    return () => cancelAnimationFrame(frame)
  })

  // Empty-drawer pages have no item renderer; the shared drawer still requires one.
  const noopItem: Snippet<[item: T, selected: boolean]> = $derived(
    itemSnippet ?? noopFallback
  )
</script>

{#snippet noopFallback(_item: unknown, _selected: boolean)}{/snippet}

{#snippet detailToggle()}
  <PanelToggle
    side="right"
    open={detailPanel.open}
    controls={detailPanelID}
    onclick={event => {
      keyboardToggle = event.detail === 0
      detailPanel.setOpen(!detailPanel.open)
    }}
  />
{/snippet}

<div
  class="page-layout"
  class:page-layout--routing={changingRoute || keyboardToggle}
  style:--main-surface={mainSurface === 'base'
    ? 'var(--color-base-100)'
    : 'var(--color-base-200)'}
>
  <SignalListDrawer
    {items}
    {emptyDrawer}
    {selectedID}
    {drawerID}
    label={drawerLabel}
    itemSnippet={noopItem}
    {itemKey}
    {onRefresh}
    {refreshPulse}
    {refreshAsideTip}
    {loading}
    {drawerChromeToolbar}
    {drawerSearch}
    {drawerControls}
    {drawerContent}
    {hideDrawer}
    {revealDrawer}
    navigationTrailing={detail ? detailToggle : undefined}
    footer={drawerFooter}
    banner={pageBanner}
  >
    <div class="page-layout__content">
      <div class="page-layout__region">
        <ResizablePanels
          layoutKey={`${routeContext.route.path}:${!!hideDrawer}:${!!detail}`}
          rightCollapsed={!detail || !detailPanel.open}
          onRightCollapsedChange={collapsed => detailPanel.setOpen(!collapsed)}
          rightPanelID={detailPanelID}
          rightWidthRem={detailWidth.rem}
          onRightWidthChange={rem => detailWidth.set(rem)}
          onLayoutMeasure={(space, stacked) =>
            detailWidth.migrateLegacy(routeContext.route.path, space, stacked)}
          defaultRightRem={PANEL_DEFAULT_REM}
          minLeftPx={remToPx(PANEL_DEFAULT_REM)}
          minRightPx={remToPx(PANEL_MIN_REM)}
          storageKey={DETAIL_SPLIT_STORAGE_KEY}
        >
          {#snippet leftPanel()}
            <div class="page-layout__main-chrome">
              <PaneHeader
                {...header?.props ?? { mode: 'title', title: '', pill: true }}
                inert={mainHeaderInert}
              />
              {@render main()}
            </div>
          {/snippet}
          {#snippet rightPanel()}
            {@render detail?.()}
          {/snippet}
        </ResizablePanels>
      </div>

      {#if pageFooter}
        <div class="page-layout__footer">
          {@render pageFooter()}
        </div>
      {/if}
    </div>
  </SignalListDrawer>
</div>

<style lang="postcss">
  @reference "../../app.css";

  /* Outer wrapper -- fills the route's flex slot in App.svelte. */
  .page-layout {
    @apply flex min-h-0 min-w-0 w-full flex-1;
  }

  /* Destination geometry is immediate; only explicit panel gestures settle. */
  .page-layout--routing :global(.signal-drawer__panel),
  .page-layout--routing :global(.signal-drawer__contents),
  .page-layout--routing :global(.panel-split),
  .page-layout--routing :global(.panel-shell),
  .page-layout--routing :global(.col-resize-bar) {
    transition: none !important;
  }

  /* Everything to the right of the drawer: a vertical column that
     stacks the content region on top and the page footer on the
     bottom. The footer (when present) spans the full content width
     -- main + detail + the resizer between them. */
  .page-layout__content {
    @apply flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden;
  }

  /* The region that hosts main (solo) or main+detail (split). Same
     shrink/min-size discipline either way so ResizablePanels can
     collapse past its children's intrinsic widths.

      The shared drawer grid owns the top inset for all three panes. This
      region supplies horizontal spacing around Main/Details. No bottom
      pad — panels sit flush against the page footer. */
  .page-layout__region {
    @apply flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden;
    padding-inline: var(--layout-gap);
  }

  /* Keep the footer surface continuous across custom page content. */
  .page-layout__footer {
    @apply shrink-0 min-w-0 bg-base-200;
  }

  .page-layout__main-chrome {
    @apply flex h-full min-h-0 min-w-0 flex-col overflow-hidden rounded-xl border-0;
    background-color: var(--main-surface);
  }
</style>
