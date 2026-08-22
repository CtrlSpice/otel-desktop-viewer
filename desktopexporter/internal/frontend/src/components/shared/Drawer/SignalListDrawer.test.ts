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
