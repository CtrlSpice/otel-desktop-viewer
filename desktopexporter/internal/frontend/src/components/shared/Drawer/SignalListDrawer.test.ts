// @vitest-environment jsdom
import { beforeAll, describe, expect, it, vi } from 'vitest'
import { createRawSnippet } from 'svelte'
import { screen, waitFor } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import SignalListDrawer from './SignalListDrawer.svelte'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'

type DrawerItem = { id: string; name: string }

const DRAWER_OPEN_KEY = 'signal-drawer:open'

const items: DrawerItem[] = [
  { id: 'trace-1', name: 'GET /checkout' },
  { id: 'trace-2', name: 'POST /orders' },
]

const itemSnippet = createRawSnippet<[DrawerItem, boolean]>(
  (item, selected) => ({
    render: () => `<p>${item().name}${selected() ? ' (selected)' : ''}</p>`,
  })
)

const keyboardItemSnippet = createRawSnippet<[DrawerItem, boolean]>(
  (item, selected) => ({
    render: () =>
      `<button type="button">${item().name}${selected() ? ' (selected)' : ''}</button>`,
  })
)

const pageContent = createRawSnippet(() => ({
  render: () => '<p>page body</p>',
}))

const focusablePageContent = createRawSnippet(() => ({
  render: () => '<button type="button">Page action</button>',
}))

const footer = createRawSnippet(() => ({
  render: () => '<p>2 traces</p>',
}))

const drawerSearch = createRawSnippet(() => ({
  render: () => '<p>search box</p>',
}))

const drawerChromeToolbar = createRawSnippet(() => ({
  render: () => '<p>sort control</p>',
}))

// Selecting a row makes the drawer ask the virtual list to scroll it into
// view, and the library calls viewport.scrollTo — which jsdom does not
// implement. Without this stand-in the scroll promise rejects.
beforeAll(() => {
  if (typeof Element.prototype.scrollTo !== 'function') {
    Element.prototype.scrollTo = () => {}
  }
})

// Instantiation expression pins the drawer's generic to DrawerItem; passing
// the bare component would collapse T to unknown and reject our snippet.
const TypedDrawer = SignalListDrawer<DrawerItem>

function drawerProps(props: Record<string, unknown> = {}) {
  return {
    items,
    selectedID: null,
    drawerID: 'traces-drawer',
    label: 'Traces',
    itemSnippet,
    children: pageContent,
    ...props,
  }
}

function renderDrawer(props: Record<string, unknown> = {}) {
  return renderWithContexts(TypedDrawer, drawerProps(props))
}

function mountedRows() {
  return Array.from(
    document.querySelectorAll<HTMLElement>('.signal-drawer__item')
  ).map(wrapper => ({
    key: wrapper.dataset.drawerItemKey,
    control: wrapper.querySelector<HTMLButtonElement>('button')!,
  }))
}

describe('SignalListDrawer', () => {
  it('renders the page content beside the drawer', () => {
    setTestUrl('/traces')
    renderDrawer()
    expect(screen.getByText('page body')).toBeInTheDocument()
  })

  it('renders a row per item using the caller-provided snippet', () => {
    setTestUrl('/traces')
    renderDrawer()
    expect(screen.getByText('GET /checkout')).toBeInTheDocument()
    expect(screen.getByText('POST /orders')).toBeInTheDocument()
  })

  it('tells the item snippet which row is selected', () => {
    setTestUrl('/traces/trace-2')
    renderDrawer({ selectedID: 'trace-2' })
    expect(screen.getByText('POST /orders (selected)')).toBeInTheDocument()
    expect(screen.getByText('GET /checkout')).toBeInTheDocument()
  })

  it('renders the footer snippet', () => {
    setTestUrl('/traces')
    renderDrawer({ footer })
    expect(screen.getByText('2 traces')).toBeInTheDocument()
  })

  it('renders the search and chrome toolbar snippets', () => {
    setTestUrl('/traces')
    renderDrawer({ drawerSearch, drawerChromeToolbar })
    expect(screen.getByText('search box')).toBeInTheDocument()
    expect(screen.getByText('sort control')).toBeInTheDocument()
  })
})

describe('SignalListDrawer empty and loading states', () => {
  it('shows an empty state naming the signal when there are no items', () => {
    setTestUrl('/traces')
    renderDrawer({ items: [] })
    expect(screen.getByRole('status')).toHaveTextContent('No traces found')
  })

  it('suggests widening the time range in the empty state', () => {
    setTestUrl('/traces')
    renderDrawer({ items: [] })
    expect(screen.getByRole('status')).toHaveTextContent(
      'Try widening the time range or clearing the search.'
    )
  })

  it('withholds the empty state while the first fetch is loading', () => {
    setTestUrl('/traces')
    renderDrawer({ items: [], loading: true })
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
  })

  it('keeps the search and time controls reachable when there are no results', () => {
    setTestUrl('/traces')
    renderDrawer({ items: [], drawerSearch })
    expect(screen.getByText('search box')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Traces' })).toBeInTheDocument()
  })
})

describe('SignalListDrawer navigation', () => {
  it('marks the destination for the signal in the URL as current', () => {
    setTestUrl('/metrics')
    renderDrawer({ label: 'Metrics' })
    expect(screen.getByRole('link', { name: 'Metrics' })).toHaveAttribute(
      'aria-current',
      'page'
    )
    expect(screen.getByRole('link', { name: 'Traces' })).not.toHaveAttribute(
      'aria-current'
    )
  })

  it('marks no signal destination current when the URL matches no signal', () => {
    setTestUrl('/')
    renderDrawer()
    for (const name of ['Traces', 'Metrics', 'Logs']) {
      expect(screen.getByRole('link', { name })).not.toHaveAttribute(
        'aria-current'
      )
    }
  })

  it('navigates to another signal when its tab is clicked', async () => {
    setTestUrl('/traces')
    renderDrawer()
    await userEvent.click(screen.getByRole('link', { name: 'Logs' }))
    expect(window.location.pathname).toBe('/logs')
  })

  it('navigates home when the home button is clicked', async () => {
    setTestUrl('/traces')
    renderDrawer()
    await userEvent.click(screen.getByRole('link', { name: 'Home' }))
    expect(window.location.pathname).toBe('/')
  })

  it('uses real hrefs that preserve the active time window', () => {
    setTestUrl('/traces?start=10&end=20&span=old')
    renderDrawer()
    expect(screen.getByRole('link', { name: 'Logs' })).toHaveAttribute(
      'href',
      '/logs?start=10&end=20'
    )
    expect(screen.getByRole('link', { name: 'Home' })).toHaveAttribute(
      'href',
      '/'
    )
  })

  it('does not intercept modified signal or Home clicks', () => {
    setTestUrl('/traces')
    renderDrawer()
    for (const name of ['Logs', 'Home']) {
      const link = screen.getByRole('link', { name })
      const event = new MouseEvent('click', {
        bubbles: true,
        cancelable: true,
        metaKey: true,
      })
      let preventedByComponent = false
      link.addEventListener(
        'click',
        clickEvent => {
          preventedByComponent = clickEvent.defaultPrevented
          clickEvent.preventDefault()
        },
        { once: true }
      )

      link.dispatchEvent(event)
      expect(preventedByComponent).toBe(false)
    }
    expect(window.location.pathname).toBe('/traces')
  })
})

describe('SignalListDrawer list keyboard navigation', () => {
  it('labels the viewport without making it a second list tab stop', async () => {
    setTestUrl('/traces')
    renderDrawer({ itemSnippet: keyboardItemSnippet })
    const viewport = screen.getByRole('region', { name: 'Traces list' })

    await waitFor(() => expect(viewport).toHaveAttribute('tabindex', '-1'))
  })

  it('keeps one row in the tab order and moves focus with list keys', async () => {
    setTestUrl('/traces')
    renderDrawer({ itemSnippet: keyboardItemSnippet })
    const user = userEvent.setup()
    const first = screen.getByRole('button', { name: 'GET /checkout' })
    const second = screen.getByRole('button', { name: 'POST /orders' })

    await waitFor(() => expect(first).toHaveAttribute('tabindex', '0'))
    expect(second).toHaveAttribute('tabindex', '-1')
    first.focus()
    await user.keyboard('{ArrowDown}')
    await waitFor(() => expect(second).toHaveFocus())
    expect(first).toHaveAttribute('tabindex', '-1')
    expect(second).toHaveAttribute('tabindex', '0')

    await user.keyboard('{Home}')
    await waitFor(() => expect(first).toHaveFocus())
  })

  it('preserves a mounted roving item through a long-list reorder', async () => {
    const longItems = Array.from({ length: 40 }, (_, index) => ({
      id: `item-${index}`,
      name: `Item ${index}`,
    }))
    setTestUrl('/traces')
    const view = renderDrawer({
      items: longItems,
      itemSnippet: keyboardItemSnippet,
    })
    const stable = await screen.findByRole('button', { name: 'Item 5' })
    stable.focus()
    await waitFor(() => expect(stable).toHaveAttribute('tabindex', '0'))

    const reordered = [
      ...longItems.slice(0, 4),
      longItems[5],
      longItems[4],
      ...longItems.slice(6),
    ]
    await view.rerender({
      component: TypedDrawer,
      componentProps: drawerProps({
        items: reordered,
        itemSnippet: keyboardItemSnippet,
      }),
    })

    const stableAfter = await screen.findByRole('button', { name: 'Item 5' })
    await waitFor(() => expect(stableAfter).toHaveAttribute('tabindex', '0'))
    expect(stableAfter).toHaveFocus()
  })

  it('chooses one mounted fallback after long-list reorder and replacement without stealing external focus', async () => {
    const longItems = Array.from({ length: 40 }, (_, index) => ({
      id: `item-${index}`,
      name: `Item ${index}`,
    }))
    setTestUrl('/traces')
    const view = renderDrawer({
      items: longItems,
      itemSnippet: keyboardItemSnippet,
    })
    const oldRoving = await screen.findByRole('button', { name: 'Item 5' })
    oldRoving.focus()
    await waitFor(() => expect(oldRoving).toHaveAttribute('tabindex', '0'))
    const external = screen.getByRole('link', { name: 'Home' })
    external.focus()

    const reordered = [
      ...longItems.filter(item => item.id !== 'item-5'),
      longItems[5],
    ]
    await view.rerender({
      component: TypedDrawer,
      componentProps: drawerProps({
        items: reordered,
        itemSnippet: keyboardItemSnippet,
      }),
    })

    await waitFor(() => {
      const rows = mountedRows()
      expect(rows.length).toBeLessThan(longItems.length)
      const tabbable = rows.filter(row => row.control.tabIndex === 0)
      expect(tabbable).toHaveLength(1)
      expect(tabbable[0].key).not.toBe('item-5')
    })
    expect(external).toHaveFocus()

    const replacement = Array.from({ length: 40 }, (_, index) => ({
      id: `replacement-${index}`,
      name: `Replacement ${index}`,
    }))
    await view.rerender({
      component: TypedDrawer,
      componentProps: drawerProps({
        items: replacement,
        itemSnippet: keyboardItemSnippet,
      }),
    })

    await waitFor(() => {
      const tabbable = mountedRows().filter(row => row.control.tabIndex === 0)
      expect(tabbable).toHaveLength(1)
      expect(tabbable[0].key).toMatch(/^replacement-/)
    })
    expect(external).toHaveFocus()
  })
})

describe('SignalListDrawer refresh control', () => {
  it('omits the refresh button when no refresh handler is given', () => {
    setTestUrl('/traces')
    renderDrawer()
    expect(
      screen.queryByRole('button', { name: 'Refresh' })
    ).not.toBeInTheDocument()
  })

  it('calls the refresh handler when the refresh button is clicked', async () => {
    setTestUrl('/traces')
    const onRefresh = vi.fn()
    renderDrawer({ onRefresh })
    await userEvent.click(screen.getByRole('button', { name: 'Refresh' }))
    expect(onRefresh).toHaveBeenCalledTimes(1)
  })

  it('announces pending data on the refresh button when new data arrives', () => {
    setTestUrl('/traces')
    renderDrawer({
      onRefresh: vi.fn(),
      refreshPulse: true,
      refreshAsideTip: '3 new traces',
    })
    expect(
      screen.getByRole('button', { name: 'Refresh — 3 new traces' })
    ).toBeInTheDocument()
    expect(screen.getByText('3 new traces')).toBeInTheDocument()
  })
})

describe('SignalListDrawer collapse behaviour', () => {
  it('opens by default when no preference is stored', () => {
    setTestUrl('/traces')
    renderDrawer()
    expect(screen.getByRole('link', { name: 'Traces' })).toBeInTheDocument()
    expect(screen.queryByLabelText('Open sidebar')).not.toBeInTheDocument()
  })

  it('collapses to the rail when the collapse control is clicked', async () => {
    setTestUrl('/traces')
    renderDrawer()
    await userEvent.click(screen.getByLabelText('Collapse sidebar'))
    expect(screen.getByLabelText('Open sidebar')).toBeInTheDocument()
    expect(document.querySelector('.signal-drawer__header')).toBeNull()
  })

  it('keeps focus on the corresponding toggle as the drawer changes state', async () => {
    setTestUrl('/traces')
    renderDrawer()
    const collapse = screen.getByRole('button', { name: 'Collapse sidebar' })
    collapse.focus()
    await userEvent.click(collapse)
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Open sidebar' })).toHaveFocus()
    )

    await userEvent.keyboard('{Enter}')
    await waitFor(() =>
      expect(
        screen.getByRole('button', { name: 'Collapse sidebar' })
      ).toHaveFocus()
    )
  })

  it('drops the list and footer when collapsed', async () => {
    setTestUrl('/traces')
    renderDrawer({ footer })
    await userEvent.click(screen.getByLabelText('Collapse sidebar'))
    expect(screen.queryByText('GET /checkout')).not.toBeInTheDocument()
    expect(screen.queryByText('2 traces')).not.toBeInTheDocument()
  })

  it('keeps the page content visible when collapsed', async () => {
    setTestUrl('/traces')
    renderDrawer()
    await userEvent.click(screen.getByLabelText('Collapse sidebar'))
    expect(screen.getByText('page body')).toBeInTheDocument()
  })

  it('remembers the collapsed preference', async () => {
    setTestUrl('/traces')
    renderDrawer()
    await userEvent.click(screen.getByLabelText('Collapse sidebar'))
    expect(localStorage.getItem(DRAWER_OPEN_KEY)).toBe('false')
  })

  it('starts collapsed when the stored preference says so', () => {
    localStorage.setItem(DRAWER_OPEN_KEY, 'false')
    setTestUrl('/traces')
    renderDrawer()
    expect(screen.getByLabelText('Open sidebar')).toBeInTheDocument()
    expect(screen.queryByText('GET /checkout')).not.toBeInTheDocument()
  })

  it('expands again from the collapsed rail', async () => {
    localStorage.setItem(DRAWER_OPEN_KEY, 'false')
    setTestUrl('/traces')
    renderDrawer()
    await userEvent.click(screen.getByLabelText('Open sidebar'))
    expect(screen.getByText('GET /checkout')).toBeInTheDocument()
    expect(localStorage.getItem(DRAWER_OPEN_KEY)).toBe('true')
  })

  it('still offers signal navigation from the collapsed rail', async () => {
    localStorage.setItem(DRAWER_OPEN_KEY, 'false')
    setTestUrl('/traces')
    renderDrawer()
    await userEvent.click(screen.getByRole('link', { name: 'Metrics' }))
    expect(window.location.pathname).toBe('/metrics')
  })
})

describe('SignalListDrawer rail-only pages', () => {
  it('puts primary navigation before page controls in the tab order', async () => {
    setTestUrl('/')
    renderDrawer({
      railOnly: true,
      items: [],
      children: focusablePageContent,
    })
    const user = userEvent.setup()

    await user.tab()
    expect(screen.getByRole('link', { name: 'Traces' })).toHaveFocus()
  })

  it('stays collapsed even when the stored preference says open', () => {
    localStorage.setItem(DRAWER_OPEN_KEY, 'true')
    setTestUrl('/')
    renderDrawer({ railOnly: true, items: [] })
    expect(document.querySelector('.signal-drawer__header')).toBeNull()
    expect(screen.getByText('page body')).toBeInTheDocument()
  })

  it('offers no control to expand the drawer', () => {
    setTestUrl('/')
    renderDrawer({ railOnly: true, items: [] })
    expect(screen.queryByLabelText('Open sidebar')).not.toBeInTheDocument()
    expect(document.querySelector('input[type="checkbox"]')).toBeDisabled()
  })

  it('shows neither the list nor the empty state', () => {
    setTestUrl('/')
    renderDrawer({ railOnly: true })
    expect(screen.queryByText('GET /checkout')).not.toBeInTheDocument()
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
  })
})

describe('SignalListDrawer drag to collapse and reopen', () => {
  // The width store is a module singleton, so each case pins its own
  // starting width instead of assuming a fresh module.
  const WIDTH_KEY = 'signal-drawer-width'

  function handle(open: boolean) {
    return screen.getByRole('separator', {
      name: open ? 'Resize the list' : 'Open the list',
    })
  }

  function drag(el: Element, fromX: number, toX: number, release = true) {
    const opts = {
      bubbles: true,
      pointerId: 1,
      pointerType: 'mouse',
      clientY: 100,
      button: 0,
      buttons: 1,
    }
    el.dispatchEvent(
      new PointerEvent('pointerdown', { ...opts, clientX: fromX })
    )
    window.dispatchEvent(
      new PointerEvent('pointermove', { ...opts, clientX: toX })
    )
    if (release) {
      window.dispatchEvent(
        new PointerEvent('pointerup', { ...opts, clientX: toX, buttons: 0 })
      )
    }
  }

  it('collapses when the drag overshoots the floor, keeping the remembered width', async () => {
    localStorage.setItem(DRAWER_OPEN_KEY, 'true')
    localStorage.setItem(WIDTH_KEY, '30')
    const { drawerWidth } = await import('@/state/drawer-width.svelte')
    drawerWidth.preview(30)
    renderDrawer()
    // jsdom has no root font size, so the component falls back to 16px/rem.
    // From 30rem, the floor is 22 and the close threshold 16: 15rem of
    // travel (240px) is decisively past it.
    drag(handle(true), 500, 500 - 15 * 16)
    expect(localStorage.getItem(DRAWER_OPEN_KEY)).toBe('false')
    // The floor the drag passed through was not committed.
    expect(localStorage.getItem(WIDTH_KEY)).toBe('30')
    expect(drawerWidth.rem).toBe(30)
    // The handle survives the collapse, relabeled for its new job.
    expect(
      await screen.findByRole('separator', { name: 'Open the list' })
    ).toBeInTheDocument()
  })

  it('reverses the collapse when the drag pulls back before release', async () => {
    localStorage.setItem(DRAWER_OPEN_KEY, 'true')
    localStorage.setItem(WIDTH_KEY, '30')
    const { drawerWidth } = await import('@/state/drawer-width.svelte')
    drawerWidth.preview(30)
    renderDrawer()
    const el = handle(true)
    const opts = {
      bubbles: true,
      pointerId: 1,
      pointerType: 'mouse',
      clientY: 100,
      button: 0,
      buttons: 1,
    }
    el.dispatchEvent(new PointerEvent('pointerdown', { ...opts, clientX: 500 }))
    // Past the close threshold, then back out to 26rem.
    window.dispatchEvent(
      new PointerEvent('pointermove', { ...opts, clientX: 500 - 15 * 16 })
    )
    window.dispatchEvent(
      new PointerEvent('pointermove', { ...opts, clientX: 500 - 4 * 16 })
    )
    window.dispatchEvent(
      new PointerEvent('pointerup', {
        ...opts,
        clientX: 500 - 4 * 16,
        buttons: 0,
      })
    )
    expect(localStorage.getItem(DRAWER_OPEN_KEY)).toBe('true')
    // An ordinary resize: the pulled-back width is committed.
    expect(localStorage.getItem(WIDTH_KEY)).toBe('26')
  })

  it('reopens at the remembered width from a short outward tug on the rail', async () => {
    localStorage.setItem(DRAWER_OPEN_KEY, 'false')
    localStorage.setItem(WIDTH_KEY, '30')
    const { drawerWidth } = await import('@/state/drawer-width.svelte')
    drawerWidth.preview(30)
    renderDrawer()
    // 2rem outward crosses the reopen threshold but never clears the
    // floor: that means "open it", not "open it at the floor".
    drag(handle(false), 60, 60 + 2 * 16)
    expect(localStorage.getItem(DRAWER_OPEN_KEY)).toBe('true')
    expect(localStorage.getItem(WIDTH_KEY)).toBe('30')
    expect(drawerWidth.rem).toBe(30)
    expect(
      await screen.findByRole('separator', { name: 'Resize the list' })
    ).toBeInTheDocument()
  })

  it('claims the Home key so resetting cannot also scroll the page', async () => {
    // Every state-mutating branch must preventDefault, or the browser
    // stacks its native handling (Home: scroll to top) on the reset.
    localStorage.setItem(DRAWER_OPEN_KEY, 'true')
    const { drawerWidth } = await import('@/state/drawer-width.svelte')
    drawerWidth.preview(30)
    renderDrawer()
    const el = handle(true)
    const ev = new KeyboardEvent('keydown', {
      key: 'Home',
      bubbles: true,
      cancelable: true,
    })
    el.dispatchEvent(ev)
    expect(ev.defaultPrevented).toBe(true)
    expect(drawerWidth.rem).toBe(28)
  })

  it('closes on ArrowLeft at the floor and reopens on ArrowRight', async () => {
    localStorage.setItem(DRAWER_OPEN_KEY, 'true')
    const { drawerWidth, MIN_DRAWER_WIDTH_REM } =
      await import('@/state/drawer-width.svelte')
    drawerWidth.preview(MIN_DRAWER_WIDTH_REM)
    renderDrawer()
    const user = userEvent.setup()
    handle(true).focus()
    await user.keyboard('{ArrowLeft}')
    expect(localStorage.getItem(DRAWER_OPEN_KEY)).toBe('false')
    handle(false).focus()
    await user.keyboard('{ArrowRight}')
    expect(localStorage.getItem(DRAWER_OPEN_KEY)).toBe('true')
  })
})
