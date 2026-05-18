<script lang="ts">
  /*
   * PageLayout: the single top-level layout used by every route.
   *
   * Owns:
   *   1. The SignalListDrawer (the left rail with nav/theme + optional
   *      item list/search/footer). Always rendered. When `items` is
   *      empty (e.g. on Home) the drawer force-collapses and acts as
   *      a thin nav rail.
   *   2. The content area: a `main` snippet (required) with an
   *      optional `detail` snippet that, when present, splits the
   *      content area into a horizontally resizable main+detail pair
   *      with the detail wrapped in standard inspector chrome.
   *
   * The page-level footer (delete + prev/next nav) lives INSIDE the
   * `main` snippet -- it acts on the main view's selection, not on
   * the detail pane. Drawer-level controls (search, sort, refresh)
   * still live in the drawer slots.
   */
  import type { Snippet } from 'svelte'
  import ResizablePanels from './ResizablePanels.svelte'
  import SignalListDrawer from './SignalListDrawer.svelte'

  type Props<T> = {
    // ── Drawer (forwarded to SignalListDrawer) ─────────────────────
    /** Items rendered in the drawer list. Pass `[]` for pages with
     * no list (Home) -- the drawer will force-collapse. */
    items: T[]
    selectedId: string | null
    drawerId: string
    /** Tooltip label on the collapsed open-drawer button. */
    drawerLabel: string
    /** Total item count for the collapsed badge (often
     * `items.length`, but may differ if filtered). */
    count?: number
    itemKey?: (item: T) => string
    /** How each row in the list renders. Optional only because
     * empty-list pages (Home) never call it. */
    itemSnippet?: Snippet<[item: T, selected: boolean]>
    onSelect?: (id: string) => void
    onRefresh?: () => void
    refreshPulse?: boolean
    refreshAsideTip?: string
    loading?: boolean
    drawerChromeToolbar?: Snippet
    drawerSearch?: Snippet
    drawerFooter?: Snippet

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

    // ── ResizablePanels config (only used when `detail` is set) ───
    defaultMainWidth?: number
    minMainWidth?: number
    minDetailWidth?: number
    /** Absolute pixel floor for the main pane. Use when fixed-size
     * chrome (e.g. a tab strip) needs guaranteed room regardless of
     * viewport width. The drag clamps to whichever min is larger. */
    minMainPx?: number
    /** Absolute pixel floor for the detail pane. */
    minDetailPx?: number
    /** Separate localStorage key for the resizable split position
     * (distinct from drawer open/closed). */
    resizableStorageKey?: string
  }

  let {
    items,
    selectedId,
    drawerId,
    drawerLabel,
    count,
    itemKey,
    itemSnippet,
    onSelect,
    onRefresh,
    refreshPulse,
    refreshAsideTip,
    loading,
    drawerChromeToolbar,
    drawerSearch,
    drawerFooter,
    main,
    detail,
    pageFooter,
    defaultMainWidth = 0.7,
    minMainWidth = 0.3,
    minDetailWidth = 0.2,
    minMainPx,
    minDetailPx,
    resizableStorageKey,
  }: Props<any> = $props()

  // No-op snippet for SignalListDrawer when items is empty -- the
  // drawer's effectivelyOpen=false guarantees this never renders,
  // but the prop is required so we satisfy the type.
  const noopItem: Snippet<[item: any, selected: boolean]> = $derived(
    itemSnippet ?? noopFallback
  )
</script>

{#snippet noopFallback(_item: any, _selected: boolean)}{/snippet}

<div class="page-layout">
  <SignalListDrawer
    {items}
    {selectedId}
    {drawerId}
    label={drawerLabel}
    count={count ?? items.length}
    itemSnippet={noopItem}
    {itemKey}
    {onSelect}
    {onRefresh}
    {refreshPulse}
    {refreshAsideTip}
    {loading}
    {drawerChromeToolbar}
    {drawerSearch}
    footer={drawerFooter}
  >
    <div class="page-layout__content">
      <div class="page-layout__region">
        {#if detail}
          <ResizablePanels
            defaultLeftWidth={defaultMainWidth}
            minLeftWidth={minMainWidth}
            minRightWidth={minDetailWidth}
            minLeftPx={minMainPx}
            minRightPx={minDetailPx}
            storageKey={resizableStorageKey}
          >
            {#snippet leftPanel()}
              <div class="page-layout__main-chrome">
                {@render main()}
              </div>
            {/snippet}
            {#snippet rightPanel()}
              <div class="page-layout__detail-chrome">
                {@render detail()}
              </div>
            {/snippet}
          </ResizablePanels>
        {:else}
          <div class="page-layout__main-chrome">
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
  @reference "../app.css";

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

  /* Page-level footer strip. Stays anchored at the bottom; never
     contributes to the resizable region's height budget. The
     SignalFooter (or whatever the page slots in) brings its own
     border-top, which is now the seam between content and footer. */
  .page-layout__footer {
    @apply shrink-0 min-w-0;
  }

  /* Inspector chrome for the detail pane. Elevated card surface with
     rounded corners so it reads as a distinct panel against the
     base-100 page background. */
  .page-layout__detail-chrome {
    @apply flex h-full min-h-0 min-w-0 flex-col overflow-hidden rounded-xl bg-base-200 border border-base-300;
  }

  .page-layout__main-chrome {
    @apply flex h-full min-h-0 min-w-0 flex-col overflow-hidden rounded-xl bg-base-200 border border-base-300;
  }
</style>
