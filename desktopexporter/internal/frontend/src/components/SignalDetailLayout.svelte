<script lang="ts">
  /*
   * SignalDetailLayout: the shared scaffolding for signal-page detail
   * areas. The left "main" pane owns the data presentation AND the
   * navigation/delete affordances (footer); the right "detail" pane
   * is an optional, mostly-read-only inspector of whatever the main
   * pane currently has selected.
   *
   * When `detail` is provided, the layout renders a horizontally
   * resizable split (ResizablePanels) with the detail pane chrome-
   * wrapped to match the trace pattern. When `detail` is undefined
   * the layout renders main full-width with no chrome change -- this
   * is the "logs has nothing to inspect beyond the row itself" case.
   *
   * Footers (delete + prev/next) live INSIDE the main pane via its
   * own footer snippet -- this layout doesn't own the footer because
   * the footer is conceptually part of the main view (it acts on
   * the main view's selection state, not on whatever the detail
   * pane is showing).
   */
  import type { Snippet } from 'svelte'
  import ResizablePanels from './ResizablePanels.svelte'

  type Props = {
    /** The primary view: logs table, trace waterfall, metric chart.
     * Required. This pane owns selection state and the footer. */
    main: Snippet
    /** Optional read-only inspector of whatever main has selected.
     * When omitted, main takes the full width with no chrome change.
     * When present, this pane is wrapped in the standard detail
     * chrome (rounded card + border + frosted bg + shadow). */
    detail?: Snippet
    /** Initial split ratio when detail is rendered. Defaults to 0.7
     * (matches trace's 70/30) so the main pane gets the lion's share
     * of horizontal space. Persisted via storageKey if provided. */
    defaultMainWidth?: number
    minMainWidth?: number
    minDetailWidth?: number
    /** localStorage key for the user's resized width. Optional --
     * pages without long-lived sessions can omit it and the layout
     * will reset to defaultMainWidth on every mount. */
    storageKey?: string
  }

  let {
    main,
    detail,
    defaultMainWidth = 0.7,
    minMainWidth = 0.3,
    minDetailWidth = 0.2,
    storageKey,
  }: Props = $props()
</script>

{#if detail}
  <div class="signal-detail-layout">
    <ResizablePanels
      defaultLeftWidth={defaultMainWidth}
      minLeftWidth={minMainWidth}
      minRightWidth={minDetailWidth}
      {storageKey}
    >
      {#snippet leftPanel()}
        {@render main()}
      {/snippet}
      {#snippet rightPanel()}
        <div class="signal-detail-layout__detail-chrome">
          {@render detail()}
        </div>
      {/snippet}
    </ResizablePanels>
  </div>
{:else}
  <div class="signal-detail-layout signal-detail-layout--solo">
    {@render main()}
  </div>
{/if}

<style lang="postcss">
  @reference "../app.css";

  /* Outer wrapper: lays out as a single flex container that fills its
     parent's remaining space. min-h-0 / min-w-0 are critical -- without
     them the inner ResizablePanels can't shrink past its content and
     the page-level scroll engages incorrectly. */
  .signal-detail-layout {
    @apply flex-1 min-h-0 min-w-0;
  }

  /* Solo mode (no detail): the main pane is the only thing here, so
     we need to ensure the chrome story matches the split-mode contract
     -- main is responsible for its own chrome in BOTH modes. The
     wrapper just provides the same shrink/min-size discipline. */
  .signal-detail-layout--solo {
    @apply flex flex-col overflow-hidden;
  }

  /* Standard detail-pane chrome. Cribbed from the trace + logs pages
     which independently arrived at the same surface treatment, so we
     centralise it here. The detail pane is always a "card" floating
     to the right of the main view. */
  .signal-detail-layout__detail-chrome {
    @apply h-full overflow-hidden rounded-xl border border-base-300/70 bg-base-100/80 shadow-surface-sm backdrop-blur-sm;
  }
</style>
