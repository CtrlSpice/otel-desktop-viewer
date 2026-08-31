<script module lang="ts">
  /*
   * PaneHeader: the single header strip used by every pane in the
   * unified layout. Four modes, one visual contract.
   *
   * Modes
   *   • title       — plain bold label on the bar background, no tabs.
   *                   Use for panes with no tab navigation (e.g. "Fields",
   *                   "Timeseries", "Log Record").
   *   • tabs        — context-aware tab strip. Route navigation highlights
   *                   the active destination with a capsule; local tabs use
   *                   an underline. Use when the pane has 2+ views.
   *   • title-tabs  — flat title on the left + local tabs on the right.
   *                   Use when the pane has a stable label AND tabs that
   *                   switch a sub-view inside the same pane.
   *   • toolbar     — chrome strip with no title or tabs. Use the
   *                   `right` snippet for a full-width control row (e.g.
   *                   time-range preset pills in the datetime popover).
   *
   * tabLayout (for `tabs` and `title-tabs`)
   *   • 'left'   — tabs pack to the start at their intrinsic width and
   *                a flexible trail fills the remaining space. Use for
   *                primary nav strips (e.g. the drawer) where a `right`
   *                slot needs to be pushed all the way to the edge.
   *   • 'right'  — tabs pack to the end; a lead spacer pushes them over.
   *                Use with `title-tabs` (title left, tabs right). No trail.
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
   *   The header is bg-base-300. Selection is drawn inside each tab without
   *   manufacturing borders around or between the header and pane body.
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
    /** Route destination; present only when the strip is navigation. */
    href?: string
  }

  export type PaneNavigationTab = PaneTab & {
    /** Real destination used when the strip is route navigation. */
    href: string
  }

  /** Stable ID shared by a local tab and its associated tabpanel. */
  export function paneTabID(panelID: string, tabID: string): string {
    return `${panelID}-tab-${tabID}`
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
    /** Optional right-aligned controls on the second header row (below
     *  the title). Renders beside `timeRange` / `timestampMs` when
     *  those are set, or alone on that row when they are not. */
    metaRight?: Snippet
  }

  type TitleProps = CommonProps & {
    mode: 'title'
    title: string
    /** Optional service name shown as "(service)" after the title. */
    subtitle?: string
  }

  /** Tab strip layout. Defaults to 'left'. Title-tabs defaults to 'right'. */
  export type PaneTabLayout = 'left' | 'equal' | 'right'

  type TabsProps = CommonProps & {
    mode: 'tabs'
    tabs: PaneTab[]
    activeID: string
    onSelect: (id: string, event?: MouseEvent) => void
    tabLayout?: PaneTabLayout
    /** Route navigation renders real links instead of ARIA tabs. */
    navigation?: boolean
    /** ID of the local tabpanel controlled by every tab in this strip. */
    tabPanelID?: string
  }

  type TitleTabsProps = CommonProps & {
    mode: 'title-tabs'
    title: string
    /** Optional service name shown as "(service)" after the title. */
    subtitle?: string
    tabs: PaneTab[]
    activeID: string
    onSelect: (id: string, event?: MouseEvent) => void
    tabLayout?: PaneTabLayout
    /** ID of the local tabpanel controlled by every tab in this strip. */
    tabPanelID: string
  }

  type ToolbarProps = CommonProps & {
    mode: 'toolbar'
    ariaLabel: string
  }

  export type PaneHeaderProps =
    TitleProps | TabsProps | TitleTabsProps | ToolbarProps
</script>

<script lang="ts">
  let props: PaneHeaderProps = $props()
  let roundedClass = $derived(
    props.rounded !== false ? 'pane-header--rounded' : 'pane-header--flush'
  )
  let tabSizeClass = $derived(props.rounded !== false ? 'tabs-sm' : '')
  let stackedClass = $derived(
    props.timeRange !== undefined ||
      props.timestampMs !== undefined ||
      props.metaRight !== undefined
      ? 'pane-header--stacked'
      : ''
  )

  function handleTablistKeydown(event: KeyboardEvent) {
    const tablist = event.currentTarget as HTMLElement
    const target = (event.target as Element | null)?.closest<HTMLButtonElement>(
      '[role="tab"]'
    )
    if (!target || !tablist.contains(target)) return

    const tabs = Array.from(
      tablist.querySelectorAll<HTMLButtonElement>('[role="tab"]:not(:disabled)')
    )
    const currentIndex = tabs.indexOf(target)
    if (currentIndex < 0 || tabs.length === 0) return

    let nextIndex: number
    if (event.key === 'ArrowRight') {
      nextIndex = (currentIndex + 1) % tabs.length
    } else if (event.key === 'ArrowLeft') {
      nextIndex = (currentIndex - 1 + tabs.length) % tabs.length
    } else if (event.key === 'Home') {
      nextIndex = 0
    } else if (event.key === 'End') {
      nextIndex = tabs.length - 1
    } else {
      return
    }

    event.preventDefault()
    const next = tabs[nextIndex]
    next.focus()
    next.click()
  }

  function tabbableTabID(
    tabs: PaneTab[],
    activeID: string
  ): string | undefined {
    return tabs.some(tab => tab.id === activeID && !tab.disabled)
      ? activeID
      : tabs.find(tab => !tab.disabled)?.id
  }
</script>

{#snippet metaRow()}
  <div class="pane-header__meta-row">
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
    {#if props.metaRight}
      <div class="pane-header__meta-right">{@render props.metaRight()}</div>
    {/if}
  </div>
{/snippet}

{#snippet badgeStrip(badges: PaneBadge[])}
  {#each badges as badge (badge.label)}
    <span class="{badge.class ?? DEFAULT_BADGE_CLASS} tabular-nums shrink-0"
      >{badge.label}</span
    >
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
  activeID: string,
  onSelect: (id: string, event?: MouseEvent) => void,
  ariaLabel: string,
  layout: PaneTabLayout,
  navigation = false,
  tabPanelID?: string
)}
  <div class="pane-header__tab-scroll pane-header__tab-scroll--{layout}">
    <div
      role={navigation ? 'navigation' : 'tablist'}
      aria-label={ariaLabel}
      class="tabs {tabSizeClass} pane-header__tabs pane-header__tabs--{layout}"
      onkeydown={navigation ? undefined : handleTablistKeydown}
    >
      {#if layout === 'right'}
        <span class="pane-header__tab-lead" aria-hidden="true"></span>
      {/if}
      {#each tabs as tab (tab.id)}
        {@const active = tab.id === activeID}
        {#if navigation && tab.href}
          <a
            href={tab.disabled ? undefined : tab.href}
            class="tab pane-header__tab gap-2 whitespace-nowrap px-3 {active
              ? 'tab-active'
              : ''}"
            aria-current={active ? 'page' : undefined}
            aria-disabled={tab.disabled ? 'true' : undefined}
            tabindex={tab.disabled ? -1 : undefined}
            title={tab.label}
            onclick={event => !tab.disabled && onSelect(tab.id, event)}
          >
            {#if tab.icon}
              <span class="pane-header__tab-icon shrink-0">
                {@render tab.icon()}
              </span>
            {/if}
            <span class="pane-header__tab-label">{tab.label}</span>
            {#if tab.count !== undefined}
              <span class="badge-count">{tab.count}</span>
            {/if}
          </a>
        {:else}
          <button
            type="button"
            role="tab"
            id={tabPanelID ? paneTabID(tabPanelID, tab.id) : undefined}
            aria-controls={tabPanelID}
            class="tab pane-header__tab gap-2 whitespace-nowrap px-3 {active
              ? 'tab-active'
              : ''}"
            aria-selected={active}
            tabindex={tab.id === tabbableTabID(tabs, activeID) ? 0 : -1}
            disabled={tab.disabled}
            title={tab.label}
            onclick={event => !tab.disabled && onSelect(tab.id, event)}
          >
            {#if tab.icon}
              <span class="pane-header__tab-icon shrink-0">
                {@render tab.icon()}
              </span>
            {/if}
            <span class="pane-header__tab-label">{tab.label}</span>
            {#if tab.count !== undefined}
              <span class="badge-count">{tab.count}</span>
            {/if}
          </button>
        {/if}
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
    {#if props.timeRange !== undefined || props.timestampMs !== undefined || props.metaRight}
      {@render metaRow()}
    {/if}
  </div>
{:else if props.mode === 'tabs'}
  <div class="pane-header pane-header--tabs {roundedClass} {stackedClass}">
    <div class="pane-header__top pane-header__top--tabs">
      {@render tabStrip(
        props.tabs,
        props.activeID,
        props.onSelect,
        props.ariaLabel ?? 'Pane tabs',
        props.tabLayout ?? 'left',
        props.navigation ?? false,
        props.tabPanelID
      )}
      {#if props.right}
        <div class="pane-header__right">{@render props.right()}</div>
      {/if}
    </div>
    {#if props.timeRange !== undefined || props.timestampMs !== undefined || props.metaRight}
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
        {#if props.subtitle?.trim()}
          <span class="pane-header__subtitle">({props.subtitle.trim()})</span>
        {/if}
        {@render badgeBlock(props.badges, props.badge)}
      </div>
      {@render tabStrip(
        props.tabs,
        props.activeID,
        props.onSelect,
        props.ariaLabel ?? `${props.title} tabs`,
        props.tabLayout ?? 'right',
        false,
        props.tabPanelID
      )}
      {#if props.right}
        <div class="pane-header__right">{@render props.right()}</div>
      {/if}
    </div>
    {#if props.timeRange !== undefined || props.timestampMs !== undefined || props.metaRight}
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

  /* Title-only: centered vertically since there are no tabs.
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

  .pane-header__meta-row {
    @apply flex w-full min-w-0 items-center justify-between gap-2 pb-1.5;
  }

  .pane-header__time-range {
    @apply min-w-0 flex-1 text-xs;
  }

  .pane-header__meta-right {
    @apply ml-auto flex shrink-0 items-center gap-2;
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
  .pane-header__badges :global(.badge),
  .pane-header__badges :global(.badge-count) {
    /* Height from daisyUI's `.badge-md`, type held a half-step under
       md so the badges grow without shouting. The second selector is
       load-bearing: `.badge-count` takes daisy's badge styles via
       @apply, so the literal `badge` class never reaches its DOM and a
       `.badge`-only rule silently missed it -- the span-count badge
       stayed xs while its neighbours upsized. */
    --size: calc(var(--size-selector, 0.25rem) * 6);
    font-size: 0.8125rem;
  }

  /* Left layout (drawer / tabs-only): tabs pack to the start at intrinsic
     width, a flexible trail fills the remaining space so a `right` slot
     can be pushed to the edge. Title-tabs uses its own rules below. */
  .pane-header:not(.pane-header--title-tabs)
    :global(.tabs.pane-header__tabs--left) {
    display: inline-flex !important;
    width: max-content !important;
    min-width: 100%;
  }

  .pane-header:not(.pane-header--title-tabs)
    :global(.tabs.pane-header__tabs--left > .tab) {
    flex: 0 0 auto !important;
    min-width: max-content;
  }

  /* Title-tabs: title on the left; tab strip fills the rest of the row
     and uses the 'right' tab layout (lead spacer + tabs). */
  .pane-header--title-tabs .pane-header__title-row--tabs {
    @apply min-w-0 max-w-[55%] shrink self-center pl-3;
    flex: 0 1 auto;
  }

  .pane-header--title-tabs .pane-header__tab-scroll {
    @apply min-h-0 min-w-0 flex-1;
    width: 0;
  }

  /* Right layout: lead spacer pushes tabs to the end. */
  .pane-header :global(.tabs.pane-header__tabs--right) {
    display: flex !important;
    width: 100% !important;
    min-width: 0 !important;
  }

  .pane-header__tab-lead {
    @apply min-w-0 flex-1 self-stretch;
  }

  .pane-header :global(.tabs.pane-header__tabs--right > .tab) {
    flex: 0 0 auto !important;
    min-width: max-content;
  }

  .pane-header__right {
    @apply ml-auto flex shrink-0 items-center gap-2;
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

  .pane-header__tab-scroll--right {
    @apply overflow-hidden;
  }

  .pane-header__tab-scroll--equal {
    @apply overflow-hidden;
  }

  /* DaisyUI's `.tabs` is `display:flex; flex-wrap:wrap` and each
     `.tab` is itself `inline-flex; flex-wrap:wrap`. Both layers
     can wrap when the pane gets narrow, which knocks the tabs out
     of alignment. Force nowrap on both for every layout. */
  .pane-header :global(.tabs.pane-header__tabs) {
    flex-wrap: nowrap !important;
    align-items: flex-end;
  }

  .pane-header :global(.tabs.pane-header__tabs > .tab) {
    flex-wrap: nowrap !important;
    white-space: nowrap;
    border: 0;
    background: transparent;
    box-shadow: none;
  }

  /* Daisy's lifted-tab borders are deliberately absent. Navigation and local
     tabs represent different actions, so their active treatments differ too. */
  .pane-header :global(.pane-header__tabs[role='navigation']) {
    @apply items-center gap-1;
  }

  .pane-header
    :global(.pane-header__tabs[role='navigation'] > .pane-header__tab) {
    @apply my-1 h-8 rounded-lg;
  }

  .pane-header
    :global(
      .pane-header__tabs[role='navigation'] > .pane-header__tab.tab-active
    ) {
    color: var(--color-primary);
    background: color-mix(in oklab, var(--color-primary) 15%, transparent);
  }

  .pane-header :global(.pane-header__tabs[role='tablist'] > .pane-header__tab) {
    border-radius: 0;
  }

  .pane-header
    :global(.pane-header__tabs[role='tablist'] > .pane-header__tab.tab-active) {
    color: var(--color-base-content);
    background: var(
      --pane-tab-active-bg,
      color-mix(in oklab, var(--color-primary) 7%, transparent)
    );
    box-shadow: inset 0 -2px var(--color-primary);
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

     The pixel min on the resize panel (minDetailPx) prevents these mins from
     ever forcing the strip wider than the pane. */
  .pane-header :global(.tabs.pane-header__tabs--equal) {
    --pane-tab-min: 7rem;
  }

  .pane-header :global(.tabs.pane-header__tabs--equal > .tab) {
    flex: 1 1 0 !important;
    min-width: var(--pane-tab-min) !important;
  }

  .pane-header__tab-trail {
    @apply min-w-4 shrink-0 grow self-stretch;
  }
</style>
