<script module lang="ts">
  // Drawer open preference is persisted in localStorage, but each
  // signal route mounts its own SignalListDrawer. Without this
  // module-level cache, the panel width transition runs on every
  // navigation even when open/closed did not change.
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
  import {
    ArrowRightIcon,
    ReloadIcon,
    BarChartHorizontalIcon,
    ChartHistogramIcon,
    LogIcon,
    HomeIcon,
  } from '@/icons'
  import ThemeToggle from '@/components/shared/ThemeToggle.svelte'
  import { startDrag, type DragHandle } from '@/components/shared/utils/drag'
  import {
    drawerWidth,
    MIN_DRAWER_WIDTH_REM,
    MAX_DRAWER_WIDTH_REM,
  } from '@/state/drawer-width.svelte'
  import DrawerNavTabs from '@/components/shared/Drawer/DrawerNavTabs.svelte'
  import {
    NAV_ITEMS,
    isNavItemActive,
  } from '@/components/shared/Drawer/DrawerNavTabs.svelte'
  import DateTimeFilter from '@/components/shared/Toolbar/DateTimeFilter.svelte'
  import PaneHeader, {
    type PaneNavigationTab,
  } from '@/components/shared/PaneHeader.svelte'
  import {
    isPlainLeftClick,
    navigate,
    navigateToSignal,
    signalHref,
    type SignalName,
  } from '@/route'
  import { getRouteContext } from '@/contexts/route-context.svelte'
  import {
    resolveNextPos,
    type KeyDelta,
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
    /** Page has no list at all (Home): force-collapse into a thin nav rail
     * and disable the expand toggle. Zero *results* on a list page should
     * NOT set this — the drawer stays open with an empty state so the
     * search and time filters remain reachable. */
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

  // Only rail-only pages (Home) force-collapse. A list page with zero
  // results keeps the drawer open and shows an empty state instead —
  // otherwise the search bar and time filter needed to broaden the query
  // are unreachable from the collapsed rail.
  let effectivelyOpen = $derived(railOnly ? false : drawerOpen)
  let showEmptyState = $derived(items.length === 0 && !loading)

  let openDrawerButton = $state<HTMLButtonElement | null>(null)
  let collapseDrawerButton = $state<HTMLButtonElement | null>(null)
  let drawerPanelID = $derived(`${drawerID}-panel`)

  async function setDrawerOpen(open: boolean) {
    if (railOnly) return
    skipWidthTransition = false
    drawerOpen = open
    persistDrawerOpen(open)
    await tick()
    const nextButton = open ? collapseDrawerButton : openDrawerButton
    nextButton?.focus()
  }

  const routeContext = getRouteContext()
  let activeNavID = $derived(
    NAV_ITEMS.find(n => isNavItemActive(n.id, routeContext.route.path))?.id ??
      ''
  )

  function handleSignalNavigation(id: string, event?: MouseEvent) {
    if (!event || !isPlainLeftClick(event)) return
    const item = NAV_ITEMS.find(n => n.id === id)
    if (!item) return
    event.preventDefault()
    navigateToSignal(item.id as SignalName)
  }

  function handleHomeNavigation(event: MouseEvent) {
    if (!isPlainLeftClick(event)) return
    event.preventDefault()
    navigate('/')
  }

  // --- auto-scroll the virtual list when the selection changes ---
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

  function handleRenderedRange(range: { start: number; end: number }) {
    renderedRange = { start: range.start, end: range.end }
  }

  const DRAWER_LIST_PAGE_STEP = 10
  const DRAWER_LIST_KEY_DELTAS: Record<string, KeyDelta> = {
    ArrowDown: { kind: 'relative', offset: 1 },
    j: { kind: 'relative', offset: 1 },
    ArrowUp: { kind: 'relative', offset: -1 },
    k: { kind: 'relative', offset: -1 },
    PageDown: { kind: 'relative', offset: DRAWER_LIST_PAGE_STEP },
    PageUp: { kind: 'relative', offset: -DRAWER_LIST_PAGE_STEP },
    Home: { kind: 'absolute', position: 'first' },
    End: { kind: 'absolute', position: 'last' },
  }

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
    const target = event.target as Element | null
    const wrapper = target?.closest<HTMLElement>('[data-drawer-item-key]')
    if (!wrapper || !drawerBodyEl?.contains(wrapper)) return null
    const focusedControl = target?.closest<HTMLElement>(
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
    const delta = DRAWER_LIST_KEY_DELTAS[event.key]
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

<div class="signal-drawer drawer drawer-open">
  <input
    id={drawerID}
    type="checkbox"
    class="drawer-toggle signal-drawer-toggle"
    checked={effectivelyOpen}
    disabled
    aria-hidden="true"
    tabindex="-1"
  />

  <div class="drawer-content min-h-0 min-w-0">
    {@render children()}
  </div>

  <div class="drawer-side">
    <div
      id={drawerPanelID}
      class="signal-drawer__panel flex h-full flex-col is-drawer-close:w-14 is-drawer-close:bg-base-300 is-drawer-open:bg-base-200"
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
      {#if !effectivelyOpen}
        <div class="signal-drawer__collapsed-rail">
          <div class="signal-drawer__collapsed-group">
            {#if railOnly}
              <span
                class="drawer-header-btn drawer-header-btn--inactive tooltip tooltip-right"
                data-tip="Waiting for data"
                aria-disabled="true"
              >
                <ArrowRightIcon
                  class="h-[17px] w-[17px] opacity-40"
                  aria-hidden="true"
                />
              </span>
            {:else}
              <!--
              data-tip matches aria-label: a chevron labelled with the signal
              name ("Traces") reads as a filter, not an expander, and a visible
              label that disagrees with the accessible name trips WCAG 2.5.3.
            -->
              <button
                bind:this={openDrawerButton}
                type="button"
                class="drawer-header-btn drawer-header-btn--inactive tooltip tooltip-right cursor-pointer"
                data-tip="Open sidebar"
                aria-label="Open sidebar"
                aria-controls={drawerPanelID}
                aria-expanded="false"
                onclick={() => setDrawerOpen(true)}
              >
                <ArrowRightIcon
                  class="h-[17px] w-[17px] animate-[spin-half_200ms_ease-out]"
                  aria-hidden="true"
                />
              </button>
            {/if}
          </div>

          <div class="separator w-8" aria-hidden="true"></div>

          <div class="signal-drawer__collapsed-group">
            <DrawerNavTabs collapsed />
          </div>

          <div class="separator w-8" aria-hidden="true"></div>
          <div class="signal-drawer__collapsed-group">
            <DateTimeFilter
              popoverAnchor="outward"
              class="drawer-header-btn drawer-header-btn--inactive shrink-0 tooltip tooltip-right"
            />
            {#if onRefresh}
              <button
                type="button"
                class="signal-drawer__refresh drawer-header-btn drawer-header-btn--inactive tooltip tooltip-right {refreshPulse &&
                refreshAsideTip
                  ? 'tooltip-secondary'
                  : ''}"
                data-tip={refreshPulse && refreshAsideTip
                  ? refreshAsideTip
                  : 'Refresh'}
                class:signal-drawer__refresh--has-new-data={refreshPulse}
                onclick={onRefresh}
                aria-label={refreshPulse
                  ? `Refresh — ${refreshAsideTip}`
                  : 'Refresh'}
              >
                {#if refreshPulse && refreshAsideTip}
                  <div class="sr-only" aria-live="polite" aria-atomic="true">
                    {refreshAsideTip}
                  </div>
                {/if}
                {#if refreshPulse}
                  <span class="signal-drawer__new-data-dot" aria-hidden="true"
                  ></span>
                {/if}
                <ReloadIcon
                  class="relative z-[1] h-[17px] w-[17px] shrink-0"
                  aria-hidden="true"
                />
              </button>
            {/if}
            <ThemeToggle
              class="drawer-header-btn drawer-header-btn--inactive tooltip tooltip-right"
            />
          </div>
        </div>
      {/if}

      <!-- Expanded: unified header panel (tabs + chrome + search + toolbar) -->
      {#if effectivelyOpen}
        {#snippet tracesIcon()}<BarChartHorizontalIcon
            class="h-[15px] w-[15px] shrink-0"
          />{/snippet}
        {#snippet metricsIcon()}<ChartHistogramIcon
            class="h-[15px] w-[15px] shrink-0"
          />{/snippet}
        {#snippet logsIcon()}<LogIcon
            class="h-[15px] w-[15px] shrink-0"
          />{/snippet}
        {@const navTabs: PaneNavigationTab[] = [
          {
            id: 'traces',
            label: 'Traces',
            icon: tracesIcon,
            href: signalHref('traces', routeContext.route.query),
          },
          {
            id: 'metrics',
            label: 'Metrics',
            icon: metricsIcon,
            href: signalHref('metrics', routeContext.route.query),
          },
          {
            id: 'logs',
            label: 'Logs',
            icon: logsIcon,
            href: signalHref('logs', routeContext.route.query),
          },
        ]}
        <div class="signal-drawer__header is-drawer-close:hidden">
          <PaneHeader
            mode="tabs"
            tabs={navTabs}
            activeID={activeNavID}
            onSelect={handleSignalNavigation}
            navigation
            rounded={false}
            ariaLabel="Primary"
          >
            {#snippet right()}
              <a
                href="/"
                class="drawer-header-btn drawer-header-btn--inactive tooltip tooltip-bottom"
                data-tip="Home"
                onclick={handleHomeNavigation}
                aria-current={routeContext.route.path === '/'
                  ? 'page'
                  : undefined}
                aria-label="Home"
              >
                <HomeIcon
                  class="h-[17px] w-[17px] shrink-0"
                  aria-hidden="true"
                />
              </a>
              <ThemeToggle
                class="drawer-header-btn drawer-header-btn--inactive tooltip tooltip-bottom"
              />
              <button
                bind:this={collapseDrawerButton}
                type="button"
                class="drawer-header-btn drawer-header-btn--inactive cursor-pointer tooltip tooltip-bottom"
                data-tip="Collapse sidebar"
                aria-label="Collapse sidebar"
                aria-controls={drawerPanelID}
                aria-expanded="true"
                onclick={() => setDrawerOpen(false)}
              >
                <ArrowRightIcon
                  class="h-[17px] w-[17px] shrink-0 transition-transform duration-200 rotate-180"
                  aria-hidden="true"
                />
              </button>
            {/snippet}
          </PaneHeader>

          {#if onRefresh || drawerSearch || drawerChromeToolbar}
            <div class="signal-drawer__search-row">
              {#if onRefresh}
                <div
                  class="shrink-0 {refreshPulse && refreshAsideTip
                    ? 'tooltip tooltip-bottom tooltip-secondary'
                    : ''}"
                  data-tip={refreshPulse && refreshAsideTip
                    ? refreshAsideTip
                    : undefined}
                >
                  {#if refreshPulse && refreshAsideTip}
                    <div class="sr-only" aria-live="polite" aria-atomic="true">
                      {refreshAsideTip}
                    </div>
                  {/if}
                  <button
                    type="button"
                    class="signal-drawer__refresh drawer-header-btn drawer-header-btn--inactive tooltip tooltip-bottom {refreshPulse &&
                    refreshAsideTip
                      ? 'tooltip-secondary'
                      : ''}"
                    class:signal-drawer__refresh--has-new-data={refreshPulse}
                    onclick={onRefresh}
                    data-tip={refreshPulse && refreshAsideTip
                      ? refreshAsideTip
                      : 'Refresh'}
                    aria-label={refreshPulse
                      ? `Refresh — ${refreshAsideTip}`
                      : 'Refresh'}
                  >
                    {#if refreshPulse}
                      <span
                        class="signal-drawer__new-data-dot"
                        aria-hidden="true"
                      ></span>
                    {/if}
                    <ReloadIcon
                      class="relative z-[1] h-[17px] w-[17px] shrink-0"
                      aria-hidden="true"
                    />
                  </button>
                </div>
              {/if}
              {#if drawerSearch}
                <div class="signal-drawer__search">
                  {@render drawerSearch()}
                </div>
              {/if}
              {#if drawerChromeToolbar}
                <div class="signal-drawer__chrome-toolbar">
                  {@render drawerChromeToolbar()}
                </div>
              {/if}
            </div>
          {/if}
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
</div>

<style lang="postcss">
  @reference "../../../app.css";

  .signal-drawer {
    @apply min-h-0 flex-1 overflow-hidden;
  }

  .signal-drawer .drawer-content {
    @apply flex flex-col;
  }

  /*
   * Nothing in the drawer chrome may clip: the icon buttons carry DaisyUI
   * tooltips, which are pseudo-elements that paint outside their trigger.
   * Clipping is not a stacking problem -- no z-index rescues a tooltip inside
   * an `overflow: hidden` ancestor, the ancestor has to stop clipping. Same
   * hazard the PaneHeader tab rule warns about.
   *
   * Three elements clip the chrome: .drawer-side, the collapsed panel (3.5rem
   * wide, clips rail tooltips sideways) and the drawer's PaneHeader (one row
   * tall, clips header tooltips downward). The outer .drawer and app shell
   * clip too, but they are full-viewport, so nothing lands outside them.
   *
   * The PaneHeader rule is scoped to this drawer on purpose -- .pane-header
   * keeps its own overflow-hidden elsewhere so long tab labels truncate
   * against it.
   *
   * This lives here rather than as `is-drawer-close:overflow-visible` on the
   * elements: that utility compiles to a `:where()` selector with zero
   * specificity, so it lost to the .drawer-side rule in this very block.
   */
  .signal-drawer :global(.drawer-side) {
    @apply h-full;
    min-height: 0;
    overflow: visible;
  }

  .signal-drawer
    :global(.drawer-toggle:not(:checked) ~ .drawer-side .signal-drawer__panel),
  .signal-drawer :global(.signal-drawer__header .pane-header) {
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

  /* ── Collapsed: open-sidebar toggle pinned to the top ── */
  @keyframes spin-half {
    from {
      transform: rotate(180deg);
    }
    to {
      transform: rotate(0deg);
    }
  }

  .signal-drawer__collapsed-rail {
    @apply flex shrink-0 flex-col items-center gap-2 px-1.5;
    padding-top: var(--layout-gap);
  }

  .signal-drawer__collapsed-group {
    @apply flex flex-col items-center gap-2;
  }

  /* ── Expanded: unified header panel ── */
  .signal-drawer__header {
    @apply flex w-full min-w-0 shrink-0 flex-col;
  }

  /* Top inset on the header bar (matches page-layout__region). */
  .signal-drawer__header :global(.pane-header.pane-header--flush) {
    @apply relative;
    padding-top: var(--layout-gap);
  }

  /* Chrome shares the tab row's centerline, not the header strip's. The
     strip is taller than the tab row (12px of top padding), so centering
     on the strip floated the buttons 6px above the tab labels. Anchoring
     to the bottom and padding by (tab row 40px - button 32px) / 2 puts
     button centers on the label line instead. */
  /* The one collapse pattern, settled after building the alternatives:
     when the drawer narrows, the tab strip slides BEHIND the pinned
     chrome and stays scrollable. Icon-only tabs, tabs-covering-chrome,
     and a two-row header were each built and rejected; sliding-under
     matches how the row badges collapse too, so the whole drawer
     speaks one language. The chrome is opaque so covered tabs are
     occluded rather than rendering through the icons, and its leading
     edge carries a fade that says the strip continues beneath. The
     fade resolves to the header surface, so while everything fits it
     paints base-300 over base-300 and is invisible -- it appears
     exactly when a tab is under it, no threshold to keep in sync. */
  .signal-drawer__header :global(.pane-header__right) {
    @apply absolute inset-y-0 right-0 z-10 flex items-end gap-2 pr-2;
    height: auto;
    margin: 0;
    padding-bottom: 4px;
    background: var(--color-base-300);
  }

  .signal-drawer__header :global(.pane-header__right)::before {
    content: '';
    position: absolute;
    right: 100%;
    top: 0;
    bottom: 0;
    width: 2rem;
    pointer-events: none;
    background: linear-gradient(to right, transparent, var(--color-base-300));
  }

  /* The chrome reserve: trailing padding equal to the chrome zone, so
     at maximum scroll the last tab sits fully clear of the icons --
     everything remains reachable, just not all at once. */
  .signal-drawer__header :global(.pane-header__tab-scroll) {
    padding-right: 7rem;
  }

  /* Refresh + new-data indicator */
  .signal-drawer__refresh {
    @apply relative;
  }

  .signal-drawer__new-data-dot {
    @apply pointer-events-none absolute bottom-0.5 right-0.5 z-[2] size-2 rounded-full bg-secondary shadow-sm ring-2 ring-base-100/90;
  }

  @keyframes signal-drawer-new-data-dot-pulse {
    0%,
    100% {
      box-shadow:
        0 0 0 1px color-mix(in oklab, var(--color-secondary) 18%, transparent),
        0 0 10px color-mix(in oklab, var(--color-secondary) 12%, transparent);
    }
    50% {
      box-shadow:
        0 0 0 1px color-mix(in oklab, var(--color-secondary) 38%, transparent),
        0 0 22px color-mix(in oklab, var(--color-secondary) 28%, transparent);
    }
  }

  .signal-drawer__refresh.signal-drawer__refresh--has-new-data:not(:hover):not(
      :focus-visible
    )
    .signal-drawer__new-data-dot {
    animation: signal-drawer-new-data-dot-pulse 2.8s ease-in-out infinite;
  }

  @media (prefers-reduced-motion: reduce) {
    .signal-drawer__refresh.signal-drawer__refresh--has-new-data
      .signal-drawer__new-data-dot {
      animation: none !important;
    }
  }

  /* ── Search + toolbar row (search · sort · time · refresh) ──
     Top pad = row bottom (pb-2) + signal-row top (py-2) → pt-4 (16px). */
  .signal-drawer__search-row {
    @apply flex min-w-0 w-full shrink-0 items-center gap-2 bg-base-200 px-2 pb-2 pt-4;
  }

  .signal-drawer__search {
    @apply min-w-0 flex-1;
  }

  .signal-drawer__chrome-toolbar {
    @apply flex shrink-0 items-center justify-end gap-2;
  }

  .signal-drawer__search
    :global(.search-editor-wrapper--drawer .search-editor__footer-actions) {
    @apply ml-auto shrink-0 gap-2;
  }

  .signal-drawer__chrome-toolbar :global(.drawer-search-panel) {
    @apply gap-0;
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
