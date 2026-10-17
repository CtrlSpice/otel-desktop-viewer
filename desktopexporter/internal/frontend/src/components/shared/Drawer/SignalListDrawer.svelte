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
  import WorkspaceToolbar from '@/components/shared/Toolbar/WorkspaceToolbar.svelte'
  import { startDrag, type DragHandle } from '@/components/shared/utils/drag'
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
    /** Page has no list (Home): hide the drawer and its toggle.
     * Empty result lists still show the drawer's empty state. */
    railOnly?: boolean
    drawerChromeToolbar?: Snippet
    drawerSearch?: Snippet
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
    railOnly = false,
    drawerChromeToolbar,
    drawerSearch,
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
  let drag: DragHandle | null = null

  function remPerPx(): number {
    const root = parseFloat(getComputedStyle(document.documentElement).fontSize)
    return Number.isFinite(root) && root > 0 ? 1 / root : 1 / 16
  }

  /* Dragging well past the floor collapses the drawer to its rail, and
   * dragging outward from the rail opens it again -- the overshoot
   * pattern IDE sidebars use. Distance, not a dwell timer: the drag
   * already expresses intent as distance, the state change previews live
   * under the pointer (pull back and it reverses), and nothing fires on
   * someone who merely paused at the floor to think. The two thresholds
   * differ so the boundary has hysteresis and cannot flicker. */
  const COLLAPSE_OVERSHOOT_REM = 6
  const REOPEN_OVERSHOOT_REM = 5

  function persistDrawerOpen(open: boolean) {
    syncDrawerOpenPreference(open)
    if (typeof localStorage !== 'undefined') {
      localStorage.setItem(DRAWER_OPEN_KEY, String(open))
    }
  }

  /* Two kinds of width change happen mid-drag and they need opposite
   * treatment. Pointer tracking must be instant -- a tween on a value
   * changing every pointermove makes the edge lag the cursor. The
   * open/rail flip is the opposite: it is a 400px jump the pointer did
   * not make, and rendering it in one frame reads as a glitch. So the
   * flip briefly lifts the instant suppression and animates, then
   * tracking snaps back to instant. */
  const SNAP_TWEEN_MS = 220
  let snapTween = $state(false)
  let snapTimer: ReturnType<typeof setTimeout> | undefined

  function beginSnapTween() {
    snapTween = true
    clearTimeout(snapTimer)
    snapTimer = setTimeout(() => {
      snapTween = false
    }, SNAP_TWEEN_MS)
  }

  function handleResizeStart(e: PointerEvent) {
    if (isResizing) return
    isResizing = true
    const wasOpen = drawerOpen
    const rememberedRem = drawerWidth.rem
    // Open drags measure from the current width. Closed drags seed just
    // under the reopen threshold, so a short outward tug opens the drawer
    // rather than demanding the pointer travel the whole missing width.
    const seedRem = wasOpen
      ? rememberedRem
      : MIN_DRAWER_WIDTH_REM - COLLAPSE_OVERSHOOT_REM
    const perPx = remPerPx()
    let lastDesired = seedRem
    drag = startDrag(e, {
      axis: 'x',
      onMove: delta => {
        // Unclamped: the store pins the width at the floor, and the pixels
        // past it are what measure intent to close or open.
        lastDesired = seedRem + delta * perPx
        drawerWidth.preview(lastDesired)
        const overshoot = MIN_DRAWER_WIDTH_REM - lastDesired
        if (drawerOpen && overshoot > COLLAPSE_OVERSHOOT_REM) {
          drawerOpen = false
          beginSnapTween()
        } else if (!drawerOpen && overshoot < REOPEN_OVERSHOOT_REM) {
          drawerOpen = true
          beginSnapTween()
        }
      },
      onEnd: () => {
        isResizing = false
        drag = null
        if (!drawerOpen) {
          // Ends closed -- whether it started that way or was closed by
          // this drag, the remembered width survives uncommitted:
          // reopening should restore the width the person actually chose,
          // not the floor they dragged through on the way out.
          drawerWidth.preview(rememberedRem)
          if (wasOpen) persistDrawerOpen(false)
          return
        }
        if (!wasOpen) {
          persistDrawerOpen(true)
          if (lastDesired < MIN_DRAWER_WIDTH_REM) {
            // A tug that never cleared the floor means "open it", not
            // "open it at the floor".
            drawerWidth.preview(rememberedRem)
            return
          }
        }
        drawerWidth.commit()
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
    clearTimeout(snapTimer)
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

  // Home has no list. Search and navigation stay mounted above the drawer.
  let effectivelyOpen = $derived(railOnly ? false : drawerOpen)
  let showEmptyState = $derived(items.length === 0 && !loading)

  let drawerPanelID = $derived(`${drawerID}-panel`)

  function setDrawerOpen(open: boolean) {
    if (railOnly) return
    skipWidthTransition = false
    drawerOpen = open
    persistDrawerOpen(open)
  }

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
    if (!effectivelyOpen || showEmptyState || !body) return

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
    if (!effectivelyOpen) {
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
    if (!effectivelyOpen || !vlistRef || !id) return
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
    drawerOpen={effectivelyOpen}
    {drawerPanelID}
    hasDrawer={!railOnly}
    onToggleDrawer={() => setDrawerOpen(!effectivelyOpen)}
    search={drawerSearch}
    controls={drawerChromeToolbar}
    {onRefresh}
    {refreshPulse}
    {refreshAsideTip}
  />
  <div
    class="signal-drawer drawer drawer-open"
    class:signal-drawer--no-list={railOnly}
  >
    <input
      id={drawerID}
      type="checkbox"
      class="drawer-toggle signal-drawer-toggle"
      checked={effectivelyOpen}
      disabled
      aria-hidden="true"
      tabindex="-1"
    />

    <div class="drawer-side">
      <div
        id={drawerPanelID}
        class="signal-drawer__panel flex h-full flex-col is-drawer-close:w-2 is-drawer-open:bg-base-200"
        class:signal-drawer__panel--instant={(skipWidthTransition ||
          isResizing) &&
          !snapTween}
        class:signal-drawer__panel--snap={snapTween}
        style={effectivelyOpen ? `width: ${drawerWidth.rem}rem` : undefined}
      >
        {#if !railOnly}
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
            aria-label={effectivelyOpen ? 'Resize the list' : 'Open the list'}
            aria-valuenow={Math.round(drawerWidth.rem)}
            aria-valuemin={MIN_DRAWER_WIDTH_REM}
            aria-valuemax={MAX_DRAWER_WIDTH_REM}
            tabindex="0"
          >
            <div class="col-resize-bar__line"></div>
          </div>
        {/if}
        <!-- Expanded: list (unmounted when collapsed so footer/count cannot leak) -->
        {#if effectivelyOpen}
          <!-- svelte-ignore a11y_no_static_element_interactions -->
          <div
            class="signal-drawer__body"
            bind:this={drawerBodyEl}
            onfocusin={handleListFocusin}
            onkeydown={handleListKeydown}
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
        {#if effectivelyOpen && footer}
          <div class="signal-drawer__footer">
            {@render footer()}
          </div>
        {/if}
      </div>
    </div>

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

  .signal-drawer--no-list :global(.drawer-side) {
    display: none;
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
    @apply relative transition-[width] duration-200;
    border-right: 1px solid
      color-mix(in oklab, var(--color-base-300) 70%, transparent);
  }

  .signal-drawer__panel--instant {
    transition: none !important;
  }

  /* The open/rail flip mid-drag: a fast, decisive deceleration. Duration
     matches SNAP_TWEEN_MS in the script, which lifts --instant for
     exactly this long before pointer tracking goes back to instant. */
  .signal-drawer__panel--snap {
    transition: width 220ms cubic-bezier(0.2, 0, 0, 1) !important;
  }

  /* Sits on the panel's trailing edge, over the border it replaces as the
     grab target. The shared .col-resize-bar supplies the cursor, hit width
     and line; this only places it. */
  .signal-drawer__resize {
    @apply absolute top-0 right-0 bottom-0 z-30;
    margin-left: 0;
    margin-right: calc(var(--resize-bar-hit-width) / -2);
  }

  @media (prefers-reduced-motion: reduce) {
    .signal-drawer__panel {
      transition: none !important;
    }
  }

  /* ── Body (list) ── */
  .signal-drawer__body {
    @apply flex-1 min-h-0 overflow-hidden;
  }

  .signal-drawer__item {
    @apply min-w-0;
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

  /* ── Footer ──
     Pinned to --app-footer-height (defined in app.css) so the
     drawer's bottom strip aligns pixel-for-pixel with the page
     footer in PageLayout. Vertical padding is replaced by
     min-height + items-center so the strip doesn't collapse around
     small controls (btn-xs) or grow with larger ones (btn-sm).
     The single direct child stretches to fill the row so consumers
     don't have to remember to add w-full themselves. */
  .signal-drawer__footer {
    @apply flex shrink-0 items-center bg-base-200 px-3;
    min-height: var(--app-footer-height);
  }

  .signal-drawer__footer > :global(*) {
    @apply min-w-0 flex-1;
  }
</style>
