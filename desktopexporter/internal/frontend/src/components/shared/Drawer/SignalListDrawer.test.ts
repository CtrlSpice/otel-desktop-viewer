// @vitest-environment jsdom
import { beforeAll, describe, expect, it, vi } from 'vitest'
import { createRawSnippet } from 'svelte'
import { screen } from '@testing-library/svelte'
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

const pageContent = createRawSnippet(() => ({
  render: () => '<p>page body</p>',
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

function renderDrawer(props: Record<string, unknown> = {}) {
  return renderWithContexts(TypedDrawer, {
    items,
    selectedID: null,
    drawerID: 'traces-drawer',
    label: 'Traces',
    itemSnippet,
    children: pageContent,
    ...props,
  })
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
    expect(screen.getByRole('tab', { name: 'Traces' })).toBeInTheDocument()
  })
})

describe('SignalListDrawer navigation', () => {
  it('marks the tab for the signal in the URL as selected', () => {
    setTestUrl('/metrics')
    renderDrawer({ label: 'Metrics' })
    expect(screen.getByRole('tab', { name: 'Metrics' })).toHaveAttribute(
      'aria-selected',
      'true'
    )
    expect(screen.getByRole('tab', { name: 'Traces' })).toHaveAttribute(
      'aria-selected',
      'false'
    )
  })

  it('falls back to the first tab when the URL matches no signal', () => {
    setTestUrl('/')
    renderDrawer()
    expect(screen.getByRole('tab', { name: 'Traces' })).toHaveAttribute(
      'aria-selected',
      'true'
    )
  })

  it('navigates to another signal when its tab is clicked', async () => {
    setTestUrl('/traces')
    renderDrawer()
    await userEvent.click(screen.getByRole('tab', { name: 'Logs' }))
    expect(window.location.pathname).toBe('/logs')
  })

  it('navigates home when the home button is clicked', async () => {
    setTestUrl('/traces')
    renderDrawer()
    await userEvent.click(screen.getByRole('button', { name: 'Home' }))
    expect(window.location.pathname).toBe('/')
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
    expect(screen.getByRole('tab', { name: 'Traces' })).toBeInTheDocument()
    expect(screen.queryByLabelText('Open sidebar')).not.toBeInTheDocument()
  })

  it('collapses to the rail when the collapse control is clicked', async () => {
    setTestUrl('/traces')
    renderDrawer()
    await userEvent.click(screen.getByLabelText('Collapse sidebar'))
    expect(screen.getByLabelText('Open sidebar')).toBeInTheDocument()
    expect(
      screen.queryByRole('tab', { name: 'Traces' })
    ).not.toBeInTheDocument()
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
    await userEvent.click(screen.getByRole('button', { name: 'Metrics' }))
    expect(window.location.pathname).toBe('/metrics')
  })
})

describe('SignalListDrawer rail-only pages', () => {
  it('stays collapsed even when the stored preference says open', () => {
    localStorage.setItem(DRAWER_OPEN_KEY, 'true')
    setTestUrl('/')
    renderDrawer({ railOnly: true, items: [] })
    expect(
      screen.queryByRole('tab', { name: 'Traces' })
    ).not.toBeInTheDocument()
    expect(screen.getByText('page body')).toBeInTheDocument()
  })

  it('offers no control to expand the drawer', () => {
    setTestUrl('/')
    renderDrawer({ railOnly: true, items: [] })
    expect(screen.queryByLabelText('Open sidebar')).not.toBeInTheDocument()
    expect(screen.getByRole('checkbox')).toBeDisabled()
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
