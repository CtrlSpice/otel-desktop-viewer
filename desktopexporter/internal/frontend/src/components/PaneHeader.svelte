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

  export type PaneTab = {
    id: string
    label: string
    /** Optional leading icon snippet. Receives no args. */
    icon?: Snippet
    /** Optional trailing count badge ("3", "12"). Rendered subtly. */
    count?: number
    /** Disabled tabs render greyed out and ignore clicks. */
    disabled?: boolean
  }

  type CommonProps = {
    /** Optional right-aligned controls/status (badge, count, mini btn). */
    right?: Snippet
    /** Accessible label for the surrounding region. Defaults to title
     * when present; required for tabs-only mode. */
    ariaLabel?: string
  }

  type TitleProps = CommonProps & {
    mode: 'title'
    title: string
  }

  type TabsProps = CommonProps & {
    mode: 'tabs'
    tabs: PaneTab[]
    activeId: string
    onSelect: (id: string) => void
  }

  type TitleTabsProps = CommonProps & {
    mode: 'title-tabs'
    title: string
    tabs: PaneTab[]
    activeId: string
    onSelect: (id: string) => void
  }

  export type PaneHeaderProps = TitleProps | TabsProps | TitleTabsProps
</script>

<script lang="ts">
  let props: PaneHeaderProps = $props()
</script>

{#snippet tabStrip(
  tabs: PaneTab[],
  activeId: string,
  onSelect: (id: string) => void,
  ariaLabel: string
)}
  <div
    role="tablist"
    aria-label={ariaLabel}
    class="tabs tabs-lift tabs-sm w-full [--tab-border-color:var(--color-primary)]"
  >
    {#each tabs as tab (tab.id)}
      {@const active = tab.id === activeId}
      <button
        type="button"
        role="tab"
        class="tab flex-1 gap-1.5 {active ? 'tab-active [--tab-bg:var(--color-base-200)] text-primary' : ''}"
        aria-selected={active}
        disabled={tab.disabled}
        onclick={() => !tab.disabled && onSelect(tab.id)}
      >
        {#if tab.icon}
          {@render tab.icon()}
        {/if}
        {tab.label}
        {#if tab.count !== undefined}
          <span class="pane-header__tab-count">{tab.count}</span>
        {/if}
      </button>
    {/each}
  </div>
{/snippet}

{#if props.mode === 'title'}
  <div
    class="pane-header pane-header--title"
    role="region"
    aria-label={props.ariaLabel ?? props.title}
  >
    <span class="pane-header__title">{props.title}</span>
    {#if props.right}
      <div class="pane-header__right">{@render props.right()}</div>
    {/if}
  </div>
{:else if props.mode === 'tabs'}
  <div class="pane-header pane-header--tabs">
    {@render tabStrip(
      props.tabs,
      props.activeId,
      props.onSelect,
      props.ariaLabel ?? 'Pane tabs'
    )}
    {#if props.right}
      <div class="pane-header__right">{@render props.right()}</div>
    {/if}
  </div>
{:else}
  <div
    class="pane-header pane-header--title-tabs"
    role="region"
    aria-label={props.ariaLabel ?? props.title}
  >
    <span class="pane-header__title">{props.title}</span>
    {@render tabStrip(
      props.tabs,
      props.activeId,
      props.onSelect,
      props.ariaLabel ?? `${props.title} tabs`
    )}
    {#if props.right}
      <div class="pane-header__right">{@render props.right()}</div>
    {/if}
  </div>
{/if}

<style lang="postcss">
  @reference "../app.css";

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
    @apply flex shrink-0 items-end gap-2 px-0.5 pt-0.5 bg-base-300 rounded-t-xl;
  }

  /* Title-only: centered vertically since there are no lift tabs. */
  .pane-header--title {
    @apply items-center py-2;
  }

  .pane-header__title {
    @apply text-sm font-semibold tracking-tight text-base-content/80;
  }

  .pane-header__right {
    @apply ml-auto flex items-center gap-1;
  }

  .pane-header__tab-count {
    @apply text-xs tabular-nums opacity-60;
  }
</style>
