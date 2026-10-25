<script module lang="ts">
  /*
   * PaneHeader: the single header strip used by every pane in the
   * unified layout. Four modes, one visual contract.
   *
   * Modes
   *   • title       — regular title beside its controls, no tabs.
   *                   Use for panes with no tab navigation (e.g. "Fields",
   *                   "Timeseries", "Log Record").
   *   • tabs        — compact tab strip with an immediate selected fill. Use
   *                   when the pane has 2+ views or route destinations.
   *   • title-tabs  — title and badges beside a compact segmented view switch.
   *                   Use when the pane has a stable label AND tabs that
   *                   switch a sub-view inside the same pane.
   *   • toolbar     — chrome strip with no title or tabs. Use the
   *                   `right` snippet for a full-width control row (e.g.
   *                   time-range preset pills in the datetime popover).
   *
   * tabLayout (for `tabs` and `title-tabs`)
   *   • 'left'   — the tab group packs to the start, leaving space for
   *                the header's right-aligned actions.
   *   • 'right'  — the tab group packs to the end.
   *                Default for `title-tabs`.
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
   *   The header is Hyoshi (bg-base-200). Selection is drawn inside each tab without
   *   manufacturing borders around or between the header and pane body.
   */
  import type { Snippet } from 'svelte'
  import PaneTimestampHeader from '@/components/shared/PaneTimestampHeader.svelte'

  export type PaneTab = {
    id: string
    label: string
    /** Optional leading icon snippet. Receives no args. */
    icon?: Snippet
    /** Optional trailing count badge ("3", "12"). */
    count?: number
    /** Use square brackets around this count instead of parentheses. */
    countBrackets?: boolean
    /** Visible unit inside the count badge; defaults to the tab label. */
    countLabel?: string
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
    inert?: boolean
    /** Optional identity mark before the title. Actions belong in metaRight. */
    leading?: Snippet
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
    /** Main signal headers share the base-300 pill surface of detail headings. */
    pill?: boolean
    /** Optional timestamp (ms) beside the title or tabs (e.g. log record time). */
    timestampMs?: number
    /** Metadata after the title and before the controls. */
    metaLeft?: Snippet
    /** Controls between metadata and the trailing actions. */
    metaCenter?: Snippet
    /** Optional controls after metadata and tabs, before the final actions. */
    metaRight?: Snippet
  }

  type TitleProps = CommonProps & {
    mode: 'title'
    title: string
    /** Optional service name shown as "(service)" after the title. */
    subtitle?: string
    /** Exact colour owner when the subtitle also includes namespace text. */
    subtitleService?: string
  }

  /** Tab strip layout. Defaults to 'left'. Title-tabs defaults to 'right'. */
  export type PaneTabLayout = 'left' | 'equal' | 'right'

  type TabsProps = CommonProps & {
    mode: 'tabs'
    /** Render only the shared tab controls, without a pane-heading surface. */
    bare?: boolean
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
    /** Exact colour owner when the subtitle also includes namespace text. */
    subtitleService?: string
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
  import ServiceDot from './ServiceDot.svelte'
  import { handleTablistKeydown } from './utils/tablist-keyboard'
  let props: PaneHeaderProps = $props()

  // Keep tooltips outside the header at normal widths. If the controls cannot
  // fit on one row, expose native scrolling instead of overlapping another pane.
  function headerOverflow(element: HTMLDivElement) {
    const measure = () =>
      element.classList.toggle(
        'pane-header--overflow',
        element.scrollWidth > element.clientWidth + 1
      )
    const resize = new ResizeObserver(measure)
    const observe = () => {
      resize.disconnect()
      resize.observe(element)
      for (const child of element.children) resize.observe(child)
      measure()
    }
    const children = new MutationObserver(observe)
    children.observe(element, { childList: true })
    observe()
    return {
      destroy() {
        resize.disconnect()
        children.disconnect()
      },
    }
  }
  let roundedClass = $derived(
    props.pill
      ? 'pane-header--pill'
      : props.rounded !== false
        ? 'pane-header--rounded'
        : 'pane-header--flush'
  )

  function tabbableTabID(
    tabs: PaneTab[],
    activeID: string
  ): string | undefined {
    return tabs.some(tab => tab.id === activeID && !tab.disabled)
      ? activeID
      : tabs.find(tab => !tab.disabled)?.id
  }
</script>

{#snippet metaRow(showControls = true)}
  {#if props.metaLeft || props.timestampMs !== undefined || props.metaCenter || (showControls && props.metaRight)}
    <div class="pane-header__meta-row">
      {#if props.metaLeft || props.timestampMs !== undefined}<div
          class="pane-header__metadata"
        >
          {#if props.metaLeft}
            {@render props.metaLeft()}
          {:else if props.timestampMs !== undefined}
            <PaneTimestampHeader timestampMs={props.timestampMs} />
          {/if}
        </div>{/if}
      {#if props.metaCenter}
        <div class="pane-header__meta-center">{@render props.metaCenter()}</div>
      {/if}
      {#if showControls && props.metaRight}
        <div class="header-controls pane-header__meta-controls">
          {@render props.metaRight()}
        </div>
      {/if}
    </div>
  {/if}
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
  tabPanelID?: string,
  withControls = false
)}
  <div
    class="header-controls pane-header__tab-group pane-header__tab-group--{layout}"
    class:pane-header__tab-group--controls={withControls}
  >
    <div
      role={navigation ? 'navigation' : 'tablist'}
      aria-label={ariaLabel}
      class="header-tabs"
      style:--header-tab-count={tabs.length}
      onkeydown={navigation ? undefined : handleTablistKeydown}
    >
      {#each tabs as tab (tab.id)}
        {@const active = tab.id === activeID}
        {#if navigation && tab.href}
          <a
            href={tab.disabled ? undefined : tab.href}
            class="btn btn-xs header-tab"
            class:tooltip={Boolean(tab.icon)}
            class:tooltip-left={Boolean(tab.icon)}
            aria-current={active ? 'page' : undefined}
            aria-disabled={tab.disabled ? 'true' : undefined}
            tabindex={tab.disabled ? -1 : undefined}
            data-tip={tab.icon ? tab.label : undefined}
            onclick={event => !tab.disabled && onSelect(tab.id, event)}
          >
            {#if tab.icon}
              <span class="header-tab__icon shrink-0">
                {@render tab.icon()}
              </span>
            {/if}
            <span class="header-tab__label">{tab.label}</span>
            {#if tabs.some(tab => tab.count !== undefined)}
              <span
                class="badge-count header-tab__count"
                aria-hidden={tab.count === undefined}
                >{#if tab.count !== undefined}{tab.countBrackets
                    ? '['
                    : '('}{tab.count}{tab.countBrackets ? ']' : ')'}<span
                    class="sr-only"
                  >
                    {tab.countLabel ?? tab.label.toLowerCase()}</span
                  >{/if}</span
              >
            {/if}
          </a>
        {:else}
          <button
            type="button"
            role="tab"
            id={tabPanelID ? paneTabID(tabPanelID, tab.id) : undefined}
            aria-controls={tabPanelID}
            class="btn btn-xs header-tab"
            class:tooltip={Boolean(tab.icon)}
            class:tooltip-left={Boolean(tab.icon)}
            aria-selected={active}
            tabindex={tab.id === tabbableTabID(tabs, activeID) ? 0 : -1}
            disabled={tab.disabled}
            data-tip={tab.icon ? tab.label : undefined}
            onclick={event => !tab.disabled && onSelect(tab.id, event)}
          >
            {#if tab.icon}
              <span class="header-tab__icon shrink-0">
                {@render tab.icon()}
              </span>
            {/if}
            <span class="header-tab__label">{tab.label}</span>
            {#if tabs.some(tab => tab.count !== undefined)}
              <span
                class="badge-count header-tab__count"
                aria-hidden={tab.count === undefined}
                >{#if tab.count !== undefined}{tab.countBrackets
                    ? '['
                    : '('}{tab.count}{tab.countBrackets ? ']' : ')'}<span
                    class="sr-only"
                  >
                    {tab.countLabel ?? tab.label.toLowerCase()}</span
                  >{/if}</span
              >
            {/if}
          </button>
        {/if}
      {/each}
    </div>
    {#if withControls && props.metaRight}{@render props.metaRight()}{/if}
  </div>
{/snippet}

{#if props.mode === 'title' || props.mode === 'title-tabs'}
  <div
    class="pane-header pane-header--{props.mode} {roundedClass}"
    use:headerOverflow
    inert={props.inert}
    role="region"
    aria-label={props.ariaLabel ?? props.title}
  >
    <div class="pane-header__top">
      <div class="pane-header__title-row">
        {#if props.leading}
          <div class="pane-header__leading">{@render props.leading()}</div>
        {/if}
        {#if props.subtitle?.trim()}
          <ServiceDot service={props.subtitleService ?? props.subtitle} />
        {/if}
        <span class="pane-header__title" title={props.title}>{props.title}</span
        >
        {#if props.subtitle?.trim()}
          <span class="pane-header__separator" aria-hidden="true">·</span>
          <span class="pane-header__subtitle">({props.subtitle.trim()})</span>
        {/if}
        {#if props.badge || props.badges?.length}
          <span class="pane-header__separator" aria-hidden="true">·</span>
          {@render badgeBlock(props.badges, props.badge)}
        {/if}
      </div>
    </div>
    {#if props.mode === 'title-tabs'}
      {@render metaRow(false)}
      {@render tabStrip(
        props.tabs,
        props.activeID,
        props.onSelect,
        props.ariaLabel ?? `${props.title} tabs`,
        props.tabLayout ?? 'right',
        false,
        props.tabPanelID,
        true
      )}
    {:else}
      {@render metaRow()}
    {/if}
    {#if props.right}
      <div class="header-controls pane-header__right">
        {@render props.right()}
      </div>
    {/if}
  </div>
{:else if props.mode === 'tabs' && props.bare}
  {@render tabStrip(
    props.tabs,
    props.activeID,
    props.onSelect,
    props.ariaLabel ?? 'Pane tabs',
    props.tabLayout ?? 'left',
    props.navigation ?? false,
    props.tabPanelID
  )}
{:else if props.mode === 'tabs'}
  <div class="pane-header pane-header--tabs {roundedClass}" use:headerOverflow>
    <div class="pane-header__top">
      {@render tabStrip(
        props.tabs,
        props.activeID,
        props.onSelect,
        props.ariaLabel ?? 'Pane tabs',
        props.tabLayout ?? 'left',
        props.navigation ?? false,
        props.tabPanelID
      )}
    </div>
    {@render metaRow()}
    {#if props.right}
      <div class="header-controls pane-header__right">
        {@render props.right()}
      </div>
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
{/if}

<style lang="postcss">
  @reference "../../app.css";

  /* Shared theme surface; the workspace toolbar can override the fill. */
  .pane-header {
    @apply flex min-w-0 shrink-0 flex-nowrap items-center border-0;
    min-height: var(--pane-header-single-h);
    gap: var(--header-gap);
    padding: var(--header-padding) var(--header-padding-inline);
    line-height: 1.5;
    container: header / inline-size;
    background-color: var(--pane-header-bg, var(--color-base-200));
  }

  .pane-header--rounded {
    @apply rounded-t-xl;
  }

  .pane-header:global(.pane-header--overflow) {
    overflow-x: auto;
  }

  .pane-header--pill {
    @apply rounded-xl;
    min-height: var(--pane-header-single-h);
  }

  .pane-header--flush {
    border-radius: 0;
  }

  .pane-header__top {
    @apply flex min-w-24 flex-1 flex-nowrap items-center gap-2;
    min-height: var(--pane-header-row-h);
  }

  .pane-header__meta-row {
    @apply flex shrink-0 items-center gap-2;
    min-height: var(--pane-header-row-h);
  }

  .pane-header__meta-center {
    @apply flex items-center justify-center;
  }

  .pane-header__metadata {
    @apply min-w-0 whitespace-nowrap text-sm;
  }

  .pane-header--toolbar {
    @apply items-center;
  }

  .pane-header__toolbar {
    @apply flex min-w-0 w-full flex-1 items-center;
  }

  .pane-header__title-row {
    @apply flex min-w-0 flex-1 flex-nowrap items-center gap-2 overflow-hidden;
  }

  .pane-header__title {
    @apply truncate text-lg font-normal text-base-content;
  }

  .pane-header__leading {
    @apply flex shrink-0 items-center self-center;
  }

  .pane-header__subtitle {
    @apply truncate text-sm font-normal leading-normal;
    color: var(--color-subtle);
  }

  /* The title shrinks before the trailing badge group and any actions. */
  .pane-header__badges {
    @apply flex shrink-0 items-center gap-2;
  }

  .pane-header__meta-controls {
    @apply ml-auto shrink-0;
  }

  .pane-header__tab-group {
    @apply max-w-full overflow-x-auto;
  }

  .pane-header__tab-group--controls {
    @apply shrink-0 overflow-visible;
  }

  .pane-header__tab-group--left {
    @apply mr-auto;
  }

  .pane-header__tab-group--right {
    @apply ml-auto justify-end;
  }

  .pane-header__tab-group--equal {
    @apply flex-1;
  }

  .pane-header__tab-group--equal .header-tabs {
    @apply w-full;
  }

  .pane-header__right {
    @apply ml-auto shrink-0;
  }

  .pane-header__separator {
    @apply shrink-0 text-base;
    color: var(--color-subtle);
  }
</style>
