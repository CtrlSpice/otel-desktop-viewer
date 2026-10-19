<script module lang="ts">
  // Cache the persisted state across route-level drawer remounts.
  let lastOpen: boolean | undefined

  /** Skip the width tween when remounting with the same open preference. */
  function shouldSkipDrawerWidthTransition(open: boolean): boolean {
    const skip = lastOpen !== undefined && lastOpen === open
    lastOpen = open
    return skip
  }

  function syncDrawerOpenPreference(open: boolean): void {
    lastOpen = open
  }
</script>

<script lang="ts" generics="T">
  import type { Snippet } from 'svelte'
  import { onDestroy, onMount, tick } from 'svelte'
  import VirtualList from '@humanspeak/svelte-virtual-list'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { ArrowUp02Icon } from '@hugeicons/core-free-icons'
  import PanelToggle from '../PanelToggle.svelte'
  import PanelClip from '../PanelClip.svelte'
  import WorkspaceToolbar from '@/components/shared/Toolbar/WorkspaceToolbar.svelte'
  import { startDrag, type DragHandle } from '@/components/shared/utils/drag'
  import {
    remToPx,
    PANEL_CLOSE_OVERSHOOT_REM,
    resolvePanelResize,
  } from '@/state/panel-width'
  import {
    drawerWidth,
    MIN_DRAWER_WIDTH_REM,
    MAX_DRAWER_WIDTH_REM,
  } from '@/state/drawer-width.svelte'
  import {
    keyDeltaFor,
    resolveNextPos,
  } from '@/components/shared/utils/table-keyboard-nav'

  type Props<T> = {
    items: T[]
    selectedID: string | null
    drawerID: string
    label: string
    itemSnippet: Snippet<[item: T, selected: boolean]>
    itemKey?: (item: T) => string
    onRefresh?: () => void
    refreshPulse?: boolean
    /** Plain text for DaisyUI tooltip + screen reader when new data is pending */
    refreshAsideTip?: string
    /** When true, an empty list shows a loading gap instead of the empty state (initial fetch). */
    loading?: boolean
    /** Leave the drawer blank for pages with no list yet (Home).
     * Empty result lists still show the drawer's empty state. */
    emptyDrawer?: boolean
    drawerChromeToolbar?: Snippet
    drawerSearch?: Snippet
    drawerControls?: Snippet
    hideDrawer?: boolean
    navigationTrailing?: Snippet
    navigationCount?: number
    footer?: Snippet
    children: Snippet
  }

  let {
    items,
    selectedID,
    drawerID,
    label,
    itemSnippet,
    // Default assumes items carry an `id` (logs, metrics); pages whose
    // items key differently (traces: traceID) must pass itemKey.
    // SAFETY: Callers using the default itemKey contract provide items with an id;
    // trace callers supply their traceID-specific itemKey instead.
    itemKey = (item: T) => (item as { id: string }).id,
    onRefresh,
    refreshPulse = false,
    refreshAsideTip = '',
    loading = false,
    emptyDrawer = false,
    drawerChromeToolbar,
    drawerSearch,
    drawerControls,
    hideDrawer = false,
    navigationTrailing,
    navigationCount,
    footer,
    children,
  }: Props<T> = $props()

  /*
   * Drawer open/closed is a single global preference shared by every
   * signal page. Each route mounts its own SignalListDrawer instance,
   * so they don't share in-memory state -- but they all read/write the
   * same localStorage key on mount/toggle, which gives "opened on
   * Traces => still opened on Logs" behavior with no cross-component
   * plumbing.
   */
  const DRAWER_OPEN_KEY = 'signal-drawer:open'

  function loadDrawerOpen(): boolean {
    if (typeof localStorage === 'undefined') return true
    const v = localStorage.getItem(DRAWER_OPEN_KEY)
    return v === null ? true : v === 'true'
  }

  const initialDrawerOpen = loadDrawerOpen()
  let drawerOpen = $state(initialDrawerOpen)
  // Resize state. The width tween is suppressed while dragging: a 200ms
  // transition on a value changing every pointermove makes the edge lag the
  // cursor instead of tracking it.
  let isResizing = $state(false)
  let dragWidthRem = $state<number | null>(null)
  const drawerShown = $derived(drawerOpen || (dragWidthRem ?? 0) > 0)
  let drag: DragHandle | null = null

  function remPerPx(): number {
    const root = parseFloat(getComputedStyle(document.documentElement).fontSize)
    return Number.isFinite(root) && root > 0 ? 1 / root : 1 / 16
  }

  // Track the pointer below the normal width floor. Choose open/closed only
  // on release, so reversing a drag never triggers a mid-gesture jump.
  const REOPEN_THRESHOLD_REM = 1.5

  function persistDrawerOpen(open: boolean) {
    syncDrawerOpenPreference(open)
    if (typeof localStorage !== 'undefined') {
      localStorage.setItem(DRAWER_OPEN_KEY, String(open))
    }
  }

  function handleResizeStart(e: PointerEvent) {
    if (isResizing) return
    const wasOpen = drawerOpen
    const rememberedRem = drawerWidth.rem
    const perPx = remPerPx()
    // Start at the rendered edge, including when a previous settle is interrupted.
    const handle = e.currentTarget as HTMLElement
    const seedRem = handle.parentElement!.getBoundingClientRect().width * perPx
    let lastDesired = seedRem
    dragWidthRem = seedRem
    isResizing = true
    drag = startDrag(e, {
      axis: 'x',
      onMove: delta => {
        lastDesired = Math.max(
          0,
          Math.min(MAX_DRAWER_WIDTH_REM, seedRem + delta * perPx)
        )
        dragWidthRem = wasOpen
          ? resolvePanelResize(
              lastDesired,
              MIN_DRAWER_WIDTH_REM,
              MAX_DRAWER_WIDTH_REM,
              PANEL_CLOSE_OVERSHOOT_REM
            ).preview
          : lastDesired
      },
      onEnd: () => {
        isResizing = false
        drag = null
        drawerOpen = wasOpen
          ? !resolvePanelResize(
              lastDesired,
              MIN_DRAWER_WIDTH_REM,
              MAX_DRAWER_WIDTH_REM,
              PANEL_CLOSE_OVERSHOOT_REM
            ).collapsed
          : lastDesired >= REOPEN_THRESHOLD_REM
        if (drawerOpen) {
          drawerWidth.preview(
            !wasOpen && lastDesired < MIN_DRAWER_WIDTH_REM
              ? rememberedRem
              : lastDesired
          )
          drawerWidth.commit()
        }
        if (drawerOpen !== wasOpen) persistDrawerOpen(drawerOpen)
        dragWidthRem = null
      },
    })
  }

  // Keyboard resizing, because a divider that only answers to a mouse is not
  // a control. Arrows nudge, Home restores the default -- and the collapse
  // mirrors the drag: another ArrowLeft at the floor closes the drawer,
  // ArrowRight or Enter on the closed handle opens it at its remembered
  // width. The handle stays mounted in both states, so focus survives the
  // toggle.
  function handleResizeKeydown(e: KeyboardEvent) {
    if (!drawerOpen) {
      if (e.key === 'ArrowRight' || e.key === 'Enter' || e.key === 'Home') {
        drawerOpen = true
        persistDrawerOpen(true)
        e.preventDefault()
      }
      return
    }
    const step = e.shiftKey ? 4 : 1
    if (e.key === 'ArrowLeft') {
      if (drawerWidth.rem <= MIN_DRAWER_WIDTH_REM) {
        drawerOpen = false
        persistDrawerOpen(false)
        e.preventDefault()
        return
      }
      drawerWidth.preview(drawerWidth.rem - step)
    } else if (e.key === 'ArrowRight') {
      drawerWidth.preview(drawerWidth.rem + step)
    } else if (e.key === 'Home') {
      // reset() persists itself; preventDefault still matters, or the
      // browser adds its native Home handling -- a scroll jump to the
      // top -- on top of the width reset.
      drawerWidth.reset()
      e.preventDefault()
      return
    } else {
      return
    }
    e.preventDefault()
    drawerWidth.commit()
  }

  function handleResizeDblclick() {
    if (!drawerOpen) {
      drawerOpen = true
      persistDrawerOpen(true)
      return
    }
    drawerWidth.reset()
  }

  // A drag outliving its handle would leave the body cursor and text
  // selection suppressed for the rest of the session.
  onDestroy(() => {
    drag?.cancel()
  })

  // Suppress width tween when remounting across signal routes (same preference).
  let skipWidthTransition = $state(
    shouldSkipDrawerWidthTransition(initialDrawerOpen)
  )

  onMount(() => {
    requestAnimationFrame(() => {
      skipWidthTransition = false
    })
  })

  let showEmptyState = $derived(items.length === 0 && !loading)

  let drawerPanelID = $derived(`${drawerID}-panel`)

  // Only fires when `selectedID` actually changes (not on items reshuffles),
  // so the user is free to scroll the list independently.
  type VirtualListRef = {
    scroll: (options: {
      index: number
      smoothScroll?: boolean
      shouldThrowOnBounds?: boolean
      align?: 'auto' | 'top' | 'bottom' | 'nearest'
    }) => Promise<void>
  }
  let vlistRef = $state<VirtualListRef | null>(null)
  let drawerBodyEl = $state<HTMLDivElement | null>(null)
  let listScrolled = $state(false)
  let scrollTopFocused = $state(false)

  function handleListScroll(event: Event) {
    const viewport = event.target
    if (
      viewport instanceof HTMLElement &&
      viewport.classList.contains('signal-drawer__vlist-viewport')
    ) {
      listScrolled = viewport.scrollTop > 1
    }
  }

  function scrollToTop() {
    void vlistRef?.scroll({
      index: 0,
      align: 'top',
      smoothScroll: !window.matchMedia('(prefers-reduced-motion: reduce)')
        .matches,
      shouldThrowOnBounds: false,
    })
  }
  let lastScrolledSelection: string | null = null
  let preferredItemKey = $state<string | null>(null)
  let lastRovingSelection: string | null | undefined
  let renderedRange = $state({ start: 0, end: 1 })

  type NavigableItemParams = { tabIndex: number }

  function itemControl(node: HTMLElement): HTMLElement | null {
    return (
      node.querySelector<HTMLElement>(
        ':scope > button, :scope > a[href], :scope > [tabindex]'
      ) ?? node.querySelector<HTMLElement>('button, a[href], [tabindex]')
    )
  }

  function navigableDrawerItem(
    node: HTMLElement,
    initial: NavigableItemParams
  ) {
    let params = initial
    const apply = () => {
      const control = itemControl(node)
      if (!control) return
      control.tabIndex = params.tabIndex
    }
    const observer = new MutationObserver(apply)
    observer.observe(node, { childList: true, subtree: true })
    apply()

    return {
      update(next: NavigableItemParams) {
        params = next
        apply()
      },
      destroy() {
        observer.disconnect()
      },
    }
  }

  $effect(() => {
    const keys = items.map(itemKey)
    const selected = selectedID
    if (selected !== lastRovingSelection) {
      lastRovingSelection = selected
      if (selected && keys.includes(selected)) preferredItemKey = selected
    }
    if (preferredItemKey && !keys.includes(preferredItemKey)) {
      preferredItemKey = selected && keys.includes(selected) ? selected : null
    }
  })

  let rovingItemKey = $derived.by(() => {
    const start = Math.max(0, Math.min(renderedRange.start, items.length))
    const end = Math.max(start, Math.min(renderedRange.end, items.length))
    const mountedKeys = items.slice(start, end).map(itemKey)
    if (preferredItemKey && mountedKeys.includes(preferredItemKey)) {
      return preferredItemKey
    }
    if (selectedID && mountedKeys.includes(selectedID)) return selectedID
    return mountedKeys[0] ?? null
  })

  $effect(() => {
    const body = drawerBodyEl
    if (!drawerOpen || showEmptyState || !body) return

    // The virtual-list package makes its scroll viewport focusable. Rows own
    // keyboard entry here, so leaving the viewport in the tab order creates a
    // dead stop immediately before the roving summary control.
    void tick().then(() => {
      if (drawerBodyEl !== body) return
      const viewport = body.querySelector<HTMLElement>(
        '.signal-drawer__vlist-viewport'
      )
      if (viewport) viewport.tabIndex = -1
    })
  })

  function handleRenderedRange(range: { start: number; end: number }) {
    renderedRange = { start: range.start, end: range.end }
  }

  const DRAWER_LIST_PAGE_STEP = 10

  function renderedItemControl(key: string): HTMLElement | null {
    const wrappers = drawerBodyEl?.querySelectorAll<HTMLElement>(
      '[data-drawer-item-key]'
    )
    if (!wrappers) return null
    for (const wrapper of wrappers) {
      if (wrapper.dataset.drawerItemKey === key) return itemControl(wrapper)
    }
    return null
  }

  async function focusDrawerItem(index: number) {
    const item = items[index]
    if (!item) return
    const key = itemKey(item)
    preferredItemKey = key
    await tick()

    const rendered = renderedItemControl(key)
    if (rendered) {
      rendered.focus()
      void vlistRef?.scroll({
        index,
        align: 'nearest',
        smoothScroll: false,
        shouldThrowOnBounds: false,
      })
      return
    }

    await vlistRef?.scroll({
      index,
      align: 'nearest',
      smoothScroll: false,
      shouldThrowOnBounds: false,
    })
    await tick()
    renderedItemControl(key)?.focus()
  }

  function drawerItemFromEvent(event: Event): HTMLElement | null {
    const target = event.target
    if (!(target instanceof Element)) return null
    const wrapper = target.closest<HTMLElement>('[data-drawer-item-key]')
    if (!wrapper || !drawerBodyEl?.contains(wrapper)) return null
    const focusedControl = target.closest<HTMLElement>(
      'button, a[href], [tabindex]'
    )
    return focusedControl === itemControl(wrapper) ? wrapper : null
  }

  function handleListFocusin(event: FocusEvent) {
    const key = drawerItemFromEvent(event)?.dataset.drawerItemKey
    if (key) preferredItemKey = key
  }

  function handleListKeydown(event: KeyboardEvent) {
    if (event.altKey || event.ctrlKey || event.metaKey) return
    const wrapper = drawerItemFromEvent(event)
    const key = wrapper?.dataset.drawerItemKey
    const delta = keyDeltaFor(event.key, DRAWER_LIST_PAGE_STEP)
    if (!key || !delta) return

    const currentIndex = items.findIndex(item => itemKey(item) === key)
    if (currentIndex < 0 || items.length === 0) return
    event.preventDefault()
    const nextIndex = resolveNextPos(delta, currentIndex, items.length - 1)
    if (nextIndex !== currentIndex) void focusDrawerItem(nextIndex)
  }

  $effect(() => {
    if (!drawerOpen) {
      lastScrolledSelection = null
      renderedRange = { start: 0, end: 1 }
    }
  })

  // Pixels of breathing room required at top/bottom for an item to count as
  // "comfortably visible". If a partially-clipped row has at least this much
  // visible margin from the closest edge, we leave the viewport alone.
  const VISIBLE_MARGIN_PX = 24

  function isComfortablyVisible(idx: number): boolean {
    const viewport = drawerBodyEl?.querySelector<HTMLElement>(
      '.signal-drawer__vlist-viewport'
    )
    const row = viewport?.querySelector<HTMLElement>(
      `[data-original-index="${idx}"]`
    )
    if (!viewport || !row) return false
    const vRect = viewport.getBoundingClientRect()
    const rRect = row.getBoundingClientRect()
    return (
      rRect.top >= vRect.top + VISIBLE_MARGIN_PX &&
      rRect.bottom <= vRect.bottom - VISIBLE_MARGIN_PX
    )
  }

  $effect(() => {
    const id = selectedID
    if (!drawerOpen || !vlistRef || !id) return
    if (id === lastScrolledSelection) return
    const idx = items.findIndex(item => itemKey(item) === id)
    if (idx < 0) return
    lastScrolledSelection = id
    if (isComfortablyVisible(idx)) return
    void vlistRef.scroll({
      index: idx,
      align: 'auto',
      smoothScroll: true,
      shouldThrowOnBounds: false,
    })
  })
</script>

<div class="workspace-shell">
  <WorkspaceToolbar
    search={drawerSearch}
    controls={drawerChromeToolbar}
    {navigationTrailing}
    {navigationCount}
    {onRefresh}
    {refreshPulse}
    {refreshAsideTip}
  >
    {#snippet navigationLeading()}
      {#if !hideDrawer}
        <PanelToggle
          side="left"
          open={drawerOpen}
          controls={drawerPanelID}
          onclick={() => {
            drawerOpen = !drawerOpen
            persistDrawerOpen(drawerOpen)
          }}
        />
      {/if}
    {/snippet}
  </WorkspaceToolbar>
  <div class="signal-drawer drawer drawer-open">
    {#if !hideDrawer}
      <input
        id={drawerID}
        type="checkbox"
        class="drawer-toggle signal-drawer-toggle"
        checked={drawerOpen}
        disabled
        aria-hidden="true"
        tabindex="-1"
      />

      <div class="drawer-side">
        <div
          id={drawerPanelID}
          class="signal-drawer__panel flex h-full flex-col"
          class:signal-drawer__panel--instant={skipWidthTransition ||
            isResizing}
          style:width={`${dragWidthRem ?? (drawerOpen ? drawerWidth.rem : 0)}rem`}
        >
          <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
          <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
          <div
            class="col-resize-bar col-resize-bar--in-flow signal-drawer__resize"
            class:col-resize-bar--active={isResizing}
            onpointerdown={handleResizeStart}
            ondblclick={handleResizeDblclick}
            onkeydown={handleResizeKeydown}
            role="separator"
            aria-orientation="vertical"
            aria-label={drawerOpen ? 'Resize the list' : 'Open the list'}
            aria-valuenow={Math.round(drawerWidth.rem)}
            aria-valuemin={MIN_DRAWER_WIDTH_REM}
            aria-valuemax={MAX_DRAWER_WIDTH_REM}
            tabindex="0"
          >
            <div class="col-resize-bar__line"></div>
          </div>
          <div
            class="signal-drawer__contents"
            class:signal-drawer__contents--closed={!drawerShown}
            inert={!drawerShown}
            aria-hidden={!drawerShown}
          >
            <PanelClip
              minimumSize={emptyDrawer ? 0 : remToPx(MIN_DRAWER_WIDTH_REM)}
            >
              {#if !emptyDrawer && drawerControls}
                <div
                  class="signal-drawer__controls"
                  role="group"
                  aria-label={`${label} list controls`}
                >
                  {@render drawerControls()}
                </div>
              {/if}
              {#if !emptyDrawer}
                <!-- svelte-ignore a11y_no_static_element_interactions -->
                <div
                  class="signal-drawer__body"
                  bind:this={drawerBodyEl}
                  onfocusin={handleListFocusin}
                  onkeydown={handleListKeydown}
                  onscrollcapture={handleListScroll}
                >
                  {#if showEmptyState}
                    <div class="signal-drawer__empty" role="status">
                      <p class="signal-drawer__empty-title">
                        No {label.toLowerCase()} found
                      </p>
                      <p class="signal-drawer__empty-hint">
                        Try widening the time range or clearing the search.
                      </p>
                    </div>
                  {:else}
                    <VirtualList
                      bind:this={vlistRef}
                      {items}
                      {itemKey}
                      defaultEstimatedItemHeight={72}
                      bufferSize={10}
                      containerClass="signal-drawer__vlist"
                      viewportClass="signal-drawer__vlist-viewport"
                      viewportLabel={`${label} list`}
                      itemsClass="signal-drawer__vlist-items"
                      onRangeChange={handleRenderedRange}
                    >
                      {#snippet renderItem(item)}
                        {@const key = itemKey(item)}
                        <div
                          class="signal-drawer__item"
                          data-drawer-item-key={key}
                          use:navigableDrawerItem={{
                            tabIndex: key === rovingItemKey ? 0 : -1,
                          }}
                        >
                          {@render itemSnippet(item, selectedID === key)}
                        </div>
                      {/snippet}
                    </VirtualList>
                  {/if}
                </div>
              {/if}

              <!-- Expanded: footer -->
              {#if !emptyDrawer}
                <div class="signal-drawer__footer">
                  <div class="min-w-0 flex-1">{@render footer?.()}</div>
                  {#if !showEmptyState && (listScrolled || scrollTopFocused)}
                    <button
                      type="button"
                      class="navigator-scroll-top btn btn-circle btn-soft btn-primary"
                      aria-label="Scroll to top"
                      title="Scroll to top"
                      onclick={scrollToTop}
                      onfocus={() => (scrollTopFocused = true)}
                      onblur={() => (scrollTopFocused = false)}
                    >
                      <HugeiconsIcon
                        icon={ArrowUp02Icon}
                        size={20}
                        strokeWidth={1.75}
                        aria-hidden="true"
                      />
                    </button>
                  {/if}
                </div>
              {/if}
            </PanelClip>
          </div>
        </div>
      </div>
    {/if}

    <div class="drawer-content min-h-0 min-w-0">
      {@render children()}
    </div>
  </div>
</div>

<style lang="postcss">
  @reference "../../../app.css";

  .workspace-shell {
    @apply flex min-h-0 min-w-0 flex-1 flex-col;
  }

  .signal-drawer {
    @apply min-h-0 flex-1 overflow-hidden;
  }

  .signal-drawer .drawer-content {
    @apply flex flex-col;
  }

  /* Keep the resize hit target reachable just beyond the drawer edge. */
  .signal-drawer :global(.drawer-side) {
    @apply h-full;
    min-height: 0;
    overflow: visible;
  }

  .signal-drawer__panel {
    @apply relative bg-base-100;
    transition: width var(--panel-motion-duration) var(--panel-motion-easing);
  }

  .signal-drawer__contents {
    @apply flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden;
    visibility: visible;
    transition: visibility 0s;
  }

  .signal-drawer__contents--closed {
    visibility: hidden;
    transition-delay: var(--panel-motion-duration);
  }

  .signal-drawer__panel--instant {
    transition: none !important;
  }

  /* Sits on the panel's trailing edge. The shared .col-resize-bar supplies the cursor, hit width
     and line; this only places it. */
  .signal-drawer__resize {
    @apply absolute top-0 right-0 bottom-0 z-30;
    margin-left: 0;
    margin-right: calc(var(--resize-bar-hit-width) / -2);
  }

  /* ── Body (list) ── */
  .signal-drawer__controls {
    @apply mx-3 mt-3 flex min-w-0 shrink-0 items-center rounded-xl border border-base-300 bg-base-200 px-1;
    min-height: var(--pane-header-h);
  }

  .signal-drawer__body {
    @apply relative flex-1 min-h-0 overflow-hidden pt-3;
  }

  .navigator-scroll-top {
    @apply size-9 shrink-0 p-0;
  }

  .signal-drawer__item {
    @apply min-w-0 px-3;
  }

  /* ── Empty state (zero results with filters still reachable above) ── */
  .signal-drawer__empty {
    @apply flex h-full flex-col items-center justify-center gap-1 px-6 text-center;
  }

  .signal-drawer__empty-title {
    @apply text-sm font-medium text-base-content/70;
  }

  .signal-drawer__empty-hint {
    @apply text-xs text-base-content/50;
  }

  .signal-drawer__body :global(.signal-drawer__vlist) {
    @apply relative h-full w-full overflow-hidden;
  }

  .signal-drawer__body :global(.signal-drawer__vlist-viewport) {
    @apply absolute inset-0 overflow-y-scroll;
    -webkit-overflow-scrolling: touch;
    scrollbar-width: thin;
  }

  .signal-drawer__body :global(.signal-drawer__vlist-items) {
    @apply absolute left-0 top-0 w-full;
  }

  /* Vertical rhythm between cards (padding counts toward measured row height; margin does not). */
  .signal-drawer__body :global(.signal-drawer__vlist-items > div) {
    @apply pb-2;
  }

  /* Align with the page footer, including its compact padding. */
  .signal-drawer__footer {
    @apply flex shrink-0 items-center gap-2 bg-base-100 px-4 py-1;
    min-height: var(--app-footer-height);
  }
</style>
