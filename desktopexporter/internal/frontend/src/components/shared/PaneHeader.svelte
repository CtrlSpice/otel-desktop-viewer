<script module lang="ts">
  /*
   * PaneHeader: the single header strip used by every pane in the
   * unified layout. Three modes, one visual contract.
   *
 * Modes
 *   • title       — plain bold label on the bar background, no lift.
 *                   Use for panes with no tab navigation (e.g. "Fields",
 *                   "Timeseries", "Log Record").
 *   • tabs        — daisyUI tabs-lift strip. Active tab lifts into the
 *                   pane body below. Use when the pane has 2+ navigable
 *                   views.
 *   • title-tabs  — flat title on the left + lift tabs on the right.
 *                   Use when the pane has a stable label AND tabs that
 *                   switch a sub-view inside the same pane.
 *   • toolbar     — chrome strip with no title or lift tabs. Use the
 *                   `right` snippet for a full-width control row (e.g.
 *                   time-range preset pills in the datetime popover).
 *
 * tabLayout (for `tabs` and `title-tabs`)
 *   • 'left'   — tabs pack to the start at their intrinsic width and
 *                a flexible trail fills the remaining space. Use for
 *                primary nav strips (e.g. the drawer) where a `right`
 *                slot needs to be pushed all the way to the edge.
 *   • 'equal'  — tabs share the row evenly, each taking 1fr of the
 *                tablist. Use for inspector tab strips (Fields /
 *                Events / Links) so labels line up across panes.
 *
 * Right slot
 *   Optional `right` snippet for status/controls that live on the same
 *   strip as the title/tabs (counts, badges, mini buttons). Always
 *   right-aligned via the bar's own flex layout.
   *
   * Consumer contract
   *   The active lift tab merges into the surface directly below the
   *   header (bg-base-200 pane body). The header itself is bg-base-300.
   */
  import type { Snippet } from 'svelte'
  import ChartTimeRangeHeader from '@/components/metrics/Charts/ChartTimeRangeHeader.svelte'
  import PaneTimestampHeader from '@/components/shared/PaneTimestampHeader.svelte'

  export type PaneTimeRange = {
    startMs: number
    endMs: number
  }

  export type PaneTab = {
    id: string
    label: string
    /** Optional leading icon snippet. Receives no args. */
    icon?: Snippet
    /** Optional trailing count badge ("3", "12"). */
    count?: number
    /** Disabled tabs render greyed out and ignore clicks. */
    disabled?: boolean
  }

  export type PaneBadge = {
    label: string
    /** daisyUI badge classes; defaults to count badge (primary soft xs). */
    class?: string
  }

  const DEFAULT_BADGE_CLASS = 'badge-count'

  type CommonProps = {
    /** Right-aligned badges (counts, severity, offsets) as
     *  plain strings. Use for ad-hoc badges where there's no shared
     *  rendering rule. Pinned to the right edge of the strip. */
    badges?: PaneBadge[]
    /** Right-aligned badge content as a snippet. Preferred path for
     *  signal-typed badges (use <SignalBadges …/> inside) so the
     *  pane header and drawer card stay in lockstep. Rendered to
     *  the left of `right` if both are provided. */
    badge?: Snippet
    /** Optional right-aligned controls/status (mini buttons, etc.).
     *  Sits at the far right edge, after `badge` / `badges`. */
    right?: Snippet
    /** Accessible label for the surrounding region. Defaults to title
     * when present; required for tabs-only mode. */
    ariaLabel?: string
    /** Apply rounded top corners. Defaults to true. */
    rounded?: boolean
    /** Optional chart/query window shown on its own line below the
     *  title or tab strip. */
    timeRange?: PaneTimeRange
    /** Optional single timestamp (ms) on its own line below the title
     *  or tab strip (e.g. log record time). */
    timestampMs?: number
  }

  type TitleProps = CommonProps & {
    mode: 'title'
    title: string
    /** Optional service name shown as "(service)" after the title. */
    subtitle?: string
  }

  /** Tab strip layout. Defaults to 'left'. */
  export type PaneTabLayout = 'left' | 'equal'

  type TabsProps = CommonProps & {
    mode: 'tabs'
    tabs: PaneTab[]
    activeId: string
    onSelect: (id: string) => void
    tabLayout?: PaneTabLayout
  }

  type TitleTabsProps = CommonProps & {
    mode: 'title-tabs'
    title: string
    tabs: PaneTab[]
    activeId: string
    onSelect: (id: string) => void
    tabLayout?: PaneTabLayout
  }

  type ToolbarProps = CommonProps & {
    mode: 'toolbar'
    ariaLabel: string
  }

  export type PaneHeaderProps =
    | TitleProps
    | TabsProps
    | TitleTabsProps
    | ToolbarProps
</script>

<script lang="ts">
  let props: PaneHeaderProps = $props()
  let roundedClass = $derived(
    props.rounded !== false ? 'pane-header--rounded' : 'pane-header--flush'
  )
  let tabSizeClass = $derived(props.rounded !== false ? 'tabs-sm' : '')
  let stackedClass = $derived(
    props.timeRange !== undefined || props.timestampMs !== undefined
      ? 'pane-header--stacked'
      : ''
  )
</script>

{#snippet metaRow()}
  <div class="pane-header__time-range">
    {#if props.timeRange}
      <ChartTimeRangeHeader
        startMs={props.timeRange.startMs}
        endMs={props.timeRange.endMs}
      />
    {:else if props.timestampMs !== undefined}
      <PaneTimestampHeader timestampMs={props.timestampMs} />
    {/if}
  </div>
{/snippet}

{#snippet badgeStrip(badges: PaneBadge[])}
  {#each badges as badge (badge.label)}
    <span
      class="{badge.class ?? DEFAULT_BADGE_CLASS} tabular-nums shrink-0"
    >{badge.label}</span>
  {/each}
{/snippet}

{#snippet badgeBlock(
  badges: PaneBadge[] | undefined,
  badge: Snippet | undefined
)}
  {#if badge || badges?.length}
    <span class="pane-header__badges">
      {#if badges?.length}
        {@render badgeStrip(badges)}
      {/if}
      {#if badge}
        {@render badge()}
      {/if}
    </span>
  {/if}
{/snippet}

{#snippet tabStrip(
  tabs: PaneTab[],
  activeId: string,
  onSelect: (id: string) => void,
  ariaLabel: string,
  layout: PaneTabLayout
)}
  <div class="pane-header__tab-scroll pane-header__tab-scroll--{layout}">
    <div
      role="tablist"
      aria-label={ariaLabel}
      class="tabs tabs-lift {tabSizeClass} pane-header__tabs pane-header__tabs--{layout}"
    >
    {#each tabs as tab (tab.id)}
      {@const active = tab.id === activeId}
      <button
        type="button"
        role="tab"
        class="tab pane-header__tab gap-2 whitespace-nowrap px-3 {active
          ? 'tab-active [--tab-bg:var(--color-base-200)]'
          : ''}"
        aria-selected={active}
        disabled={tab.disabled}
        onclick={() => !tab.disabled && onSelect(tab.id)}
      >
        {#if tab.icon}
          {@render tab.icon()}
        {/if}
        {tab.label}
        {#if tab.count !== undefined}
          <span class="badge-count">{tab.count}</span>
        {/if}
      </button>
    {/each}
    {#if layout === 'left'}
      <span class="pane-header__tab-trail" aria-hidden="true"></span>
    {/if}
    </div>
  </div>
{/snippet}

{#if props.mode === 'title'}
  <div
    class="pane-header pane-header--title {roundedClass} {stackedClass}"
    role="region"
    aria-label={props.ariaLabel ?? props.title}
  >
    <div class="pane-header__top">
    <div class="pane-header__title-row">
      <span class="pane-header__title">{props.title}</span>
      {#if props.subtitle?.trim()}
        <span class="pane-header__subtitle">({props.subtitle.trim()})</span>
      {/if}
      {@render badgeBlock(props.badges, props.badge)}
    </div>
    {#if props.right}
      <div class="pane-header__right">{@render props.right()}</div>
    {/if}
    </div>
    {#if props.timeRange !== undefined || props.timestampMs !== undefined}
      {@render metaRow()}
    {/if}
  </div>
{:else if props.mode === 'tabs'}
  <div class="pane-header pane-header--tabs {roundedClass} {stackedClass}">
    <div class="pane-header__top pane-header__top--tabs">
      {@render tabStrip(
        props.tabs,
        props.activeId,
        props.onSelect,
        props.ariaLabel ?? 'Pane tabs',
        props.tabLayout ?? 'left'
      )}
      {#if props.right}
        <div class="pane-header__right">{@render props.right()}</div>
      {/if}
    </div>
    {#if props.timeRange !== undefined || props.timestampMs !== undefined}
      {@render metaRow()}
    {/if}
  </div>
{:else if props.mode === 'toolbar'}
  <div
    class="pane-header pane-header--toolbar {roundedClass}"
    role="toolbar"
    aria-label={props.ariaLabel}
  >
    {#if props.right}
      <div class="pane-header__toolbar">{@render props.right()}</div>
    {/if}
  </div>
{:else}
  <div
    class="pane-header pane-header--title-tabs {roundedClass} {stackedClass}"
    role="region"
    aria-label={props.ariaLabel ?? props.title}
  >
    <div class="pane-header__top pane-header__top--title-tabs">
      <div class="pane-header__title-row pane-header__title-row--tabs">
        <span class="pane-header__title">{props.title}</span>
        {@render badgeBlock(props.badges, props.badge)}
      </div>
      {@render tabStrip(
        props.tabs,
        props.activeId,
        props.onSelect,
        props.ariaLabel ?? `${props.title} tabs`,
        props.tabLayout ?? 'left'
      )}
      {#if props.right}
        <div class="pane-header__right">{@render props.right()}</div>
      {/if}
    </div>
    {#if props.timeRange !== undefined || props.timestampMs !== undefined}
      {@render metaRow()}
    {/if}
  </div>
{/if}

<style lang="postcss">
  @reference "../../app.css";

  /*
   * Outer header strip. Shares the .pane-header name with the
   * global helper in app.css (same surface concept).
   *
   * Note the local class is .pane-header here (component-scoped)
   * and shares the name with the global .pane-header in app.css on
   * purpose — both describe the same surface. We don't @apply the
   * global one because we want full control over flex layout and
   * the title/tabs/right grid; the global one is a "fill + height"
   * helper for ad-hoc consumers.
   */
  .pane-header {
    @apply flex min-w-0 shrink-0 flex-nowrap items-end gap-2 overflow-hidden px-0.5 pt-0.5 bg-base-300;
  }

  .pane-header--rounded {
    @apply rounded-t-xl;
  }

  .pane-header--flush {
    @apply pt-1;
  }

  /* Title-only: centered vertically since there are no lift tabs.
     px-3 matches FieldGroup headings and detail row inset. */
  .pane-header--title {
    @apply items-center px-3 py-2;
  }

  .pane-header--stacked {
    @apply flex-col items-stretch gap-0;
  }

  .pane-header--stacked.pane-header--title {
    @apply items-stretch py-2 pb-0;
  }

  .pane-header__top {
    @apply flex min-w-0 w-full flex-nowrap items-center gap-2;
  }

  .pane-header__top--tabs,
  .pane-header__top--title-tabs {
    @apply items-end;
  }

  .pane-header--title-tabs .pane-header__top--title-tabs {
    @apply min-w-0 flex-1;
  }

  .pane-header__time-range {
    @apply w-full pb-1.5 text-xs;
  }

  .pane-header--toolbar {
    @apply items-center px-2 py-1.5;
  }

  .pane-header__toolbar {
    @apply flex min-w-0 w-full flex-1 items-center;
  }

  .pane-header--tabs,
  .pane-header--title-tabs {
    @apply min-w-0 flex-nowrap overflow-hidden;
  }

  .pane-header__title-row {
    @apply flex min-w-0 flex-1 flex-nowrap items-baseline gap-1.5 overflow-hidden;
  }

  .pane-header__title {
    @apply truncate text-sm font-semibold tracking-tight text-base-content;
  }

  .pane-header__subtitle {
    @apply truncate text-sm font-normal leading-none;
    color: var(--color-subtle);
  }

  /* Badges sit inline after the title/subtitle, mirroring the
     drawer card's `.signal-row__badge` layout. Title row owns the
     `flex-1` truncation so a long title compresses before badges
     do. */
  .pane-header__badges {
    @apply flex shrink-0 items-center gap-1.5;
  }

  /* Header badges read at a slightly larger size than their
     drawer-card counterparts. xs is right for stacked card density;
     the header has more breathing room and benefits from sm so the
     type / severity reads at a glance. Same `<SignalBadges>` markup,
     contextually upsized via this scoped override.

     Targets the daisyUI `.badge` class globally because the badges
     originate in a child component (SignalBadges) whose classes
     Svelte's scoped selectors don't reach. Bounded to
     `.pane-header__badges` so it doesn't leak. Values mirror
     daisyUI's `.badge-sm` definition exactly. */
  .pane-header__badges :global(.badge) {
    --size: calc(var(--size-selector, 0.25rem) * 5);
    font-size: 0.75rem;
  }

  .pane-header--title-tabs .pane-header__title-row--tabs {
    @apply min-w-0 max-w-[45%] shrink pl-3;
  }

  .pane-header__right {
    @apply ml-auto flex shrink-0 items-center gap-2 px-1;
  }

  /* flex-1 + width:0 lets this shrink inside the header. Scroll
     behaviour differs by layout: 'left' allows overflow so a long
     tab list keeps its intrinsic widths and scrolls horizontally;
     'equal' clips so each tab can compress to share the row. */
  .pane-header__tab-scroll {
    @apply min-h-0 min-w-0 flex-1;
    width: 0;
  }

  .pane-header__tab-scroll--left {
    @apply overflow-x-auto overflow-y-hidden;
    scrollbar-width: thin;
  }

  .pane-header__tab-scroll--equal {
    @apply overflow-hidden;
  }

  /* DaisyUI's `.tabs` is `display:flex; flex-wrap:wrap` and each
     `.tab` is itself `inline-flex; flex-wrap:wrap`. Both layers
     can wrap when the pane gets narrow, which knocks the lift
     tabs out of alignment. Force nowrap on both for every layout. */
  .pane-header :global(.tabs.pane-header__tabs) {
    flex-wrap: nowrap !important;
    align-items: flex-end;
  }

  .pane-header :global(.tabs.pane-header__tabs > .tab) {
    flex-wrap: nowrap !important;
    white-space: nowrap;
  }

  /* Left layout: tabs pack to the start at intrinsic width, a
     flexible trail fills the remaining space so a `right` slot
     can be pushed to the edge. The strip can grow past 100% and
     scroll inside .pane-header__tab-scroll--left. */
  .pane-header :global(.tabs.pane-header__tabs--left) {
    display: inline-flex !important;
    width: max-content !important;
    min-width: 100%;
  }

  .pane-header :global(.tabs.pane-header__tabs--left > .tab) {
    flex: 0 0 auto !important;
    min-width: max-content;
  }

  /* Equal layout: each tab gets 1fr of the row. No trail; no
     horizontal scroll — labels truncate before the row overflows. */
  .pane-header :global(.tabs.pane-header__tabs--equal) {
    display: flex !important;
    width: 100% !important;
  }

  /* Each tab has a per-tab minimum so labels never crowd. Sized to
     fit our longest detail-view label ("Datapoints") with badge +
     gaps. Override at the call site with style="--pane-tab-min: …"
     if a future tab strip uses longer labels.

     Note: do NOT set `overflow: hidden` on the tab — daisyUI's
     tabs-lift draws the rounded outer corners via a ::before pseudo
     that extends past the tab box, so clipping overflow erases the
     notch. The pixel min on the resize panel (minDetailPx) is what
     prevents these mins from ever forcing the strip wider than the
     pane. */
  .pane-header :global(.tabs.pane-header__tabs--equal) {
    --pane-tab-min: 7rem;
  }

  .pane-header :global(.tabs.pane-header__tabs--equal > .tab) {
    flex: 1 1 0 !important;
    min-width: var(--pane-tab-min) !important;
  }

  .pane-header__tab-trail {
    @apply min-w-4 shrink-0 grow self-stretch border-b border-base-300;
  }
</style>
