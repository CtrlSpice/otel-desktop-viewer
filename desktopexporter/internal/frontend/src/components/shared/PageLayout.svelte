<script lang="ts" generics="T">
  /*
   * PageLayout: the single top-level layout used by every route.
   *
   * Owns:
   *   1. The workspace shell: horizontal navigation/search/actions above
   *      the resizable list drawer. `emptyDrawer` pages (Home) leave it blank;
   *      the toolbar stays available on every route and at every drawer width.
   *   2. The content area: a `main` snippet (required) with an
   *      optional `detail` snippet that, when present, splits the
   *      content area into a horizontally resizable main+detail pair.
   *
   * The page-level footer (delete + prev/next nav) lives INSIDE the
   * `main` snippet -- it acts on the main view's selection, not on
   * the detail pane. Search renders in the workspace toolbar; list controls
   * stay above the drawer's scrolling items.
   */
  import type { Snippet } from 'svelte'
  import ResizablePanels from './ResizablePanels.svelte'
  import SignalListDrawer from './Drawer/SignalListDrawer.svelte'
  import PanelToggle from './PanelToggle.svelte'
  import { detailWidth } from '@/state/detail-width.svelte'
  import { detailPanel } from '@/state/detail-panel.svelte'
  import {
    PANEL_DEFAULT_REM,
    PANEL_MIN_REM,
    remToPx,
  } from '@/state/panel-width'

  type Props<T> = {
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
    /** Current result count shown in the active navigation tab. */
    navigationCount?: number
    drawerChromeToolbar?: Snippet
    drawerSearch?: Snippet
    drawerControls?: Snippet
    drawerFooter?: Snippet
    /** Omit the list pane and its toggle without changing the saved preference. */
    hideDrawer?: boolean

    // ── Content slots ──────────────────────────────────────────────
    /** Primary view (waterfall, chart, logs table, home content). */
    main: Snippet
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

  let {
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
    navigationCount,
    drawerChromeToolbar,
    drawerSearch,
    drawerControls,
    drawerFooter,
    hideDrawer,
    main,
    detail,
    pageFooter,
    mainSurface = 'panel',
  }: Props<T> = $props()

  const detailPanelID = $derived(`${drawerID}-detail`)

  // Empty-drawer pages have no item renderer; the shared drawer still requires one.
  const noopItem: Snippet<[item: T, selected: boolean]> = $derived(
    itemSnippet ?? noopFallback
  )
</script>

{#snippet noopFallback(_item: T, _selected: boolean)}{/snippet}

{#snippet detailToggle()}
  <PanelToggle
    side="right"
    open={detailPanel.open}
    controls={detailPanelID}
    onclick={() => detailPanel.setOpen(!detailPanel.open)}
  />
{/snippet}

<div
  class="page-layout"
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
    {navigationCount}
    {drawerChromeToolbar}
    {drawerSearch}
    {drawerControls}
    {hideDrawer}
    navigationTrailing={detail ? detailToggle : undefined}
    footer={drawerFooter}
  >
    <div class="page-layout__content">
      <div class="page-layout__region">
        {#if detail}
          <ResizablePanels
            rightCollapsed={!detailPanel.open}
            onRightCollapsedChange={collapsed =>
              detailPanel.setOpen(!collapsed)}
            rightPanelID={detailPanelID}
            rightWidthRem={detailWidth.rem}
            onRightWidthChange={rem => detailWidth.set(rem)}
            defaultRightRem={PANEL_DEFAULT_REM}
            minLeftPx={remToPx(PANEL_DEFAULT_REM)}
            minRightPx={remToPx(PANEL_MIN_REM)}
            storageKey="workspace-panel-split"
          >
            {#snippet leftPanel()}
              <div
                class="page-layout__main-chrome"
                class:page-layout__main-chrome--base={mainSurface === 'base'}
              >
                {@render main()}
              </div>
            {/snippet}
            {#snippet rightPanel()}
              {@render detail()}
            {/snippet}
          </ResizablePanels>
        {:else}
          <div
            class="page-layout__main-chrome"
            class:page-layout__main-chrome--base={mainSurface === 'base'}
          >
            {@render main()}
          </div>
        {/if}
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

     Horizontal + top inset gives main+detail breathing room from the
     drawer's right edge and the top of the page. No bottom pad — panels
     sit flush against the page footer. */
  .page-layout__region {
    @apply flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden;
    padding-inline: var(--layout-gap);
    padding-top: var(--layout-gap);
  }

  /* Keep the footer surface continuous across custom page content. */
  .page-layout__footer {
    @apply shrink-0 min-w-0 bg-base-100;
  }

  .page-layout__main-chrome {
    @apply flex h-full min-h-0 min-w-0 flex-col overflow-hidden rounded-xl border border-base-300;
    background-color: var(--main-surface);
  }

  .page-layout__main-chrome--base {
    @apply border-0;
  }
</style>
