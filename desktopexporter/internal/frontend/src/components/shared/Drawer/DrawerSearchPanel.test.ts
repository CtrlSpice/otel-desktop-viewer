// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { screen, waitFor } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import DrawerSearchPanel from './DrawerSearchPanel.svelte'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'

const sortOptions = [
  { value: 'time', label: 'Start time' },
  { value: 'duration', label: 'Duration' },
  {
    value: 'spanCount',
    label: 'Span count',
    defaultDirection: 'desc' as const,
  },
]

// The popover JS API (methods + popovertarget invokers) comes from the shared
// polyfill in src/test/setup.ts, so these tests open the sort menu the way a
// user would: by clicking the trigger.

function renderPanel(props: Record<string, unknown> = {}) {
  setTestUrl('/traces')
  return renderWithContexts(DrawerSearchPanel, {
    signal: 'traces',
    sortOptions,
    sortValue: 'duration',
    sortDirection: 'desc',
    ...props,
  })
}

async function openSortMenu() {
  await userEvent.click(screen.getByRole('button', { name: /^Sort by/ }))
  return screen.getByRole('menu', { name: 'Sort by' })
}

describe('DrawerSearchPanel toolbar segment', () => {
  it('renders a toolbar of list controls', () => {
    renderPanel({ segment: 'toolbar' })
    expect(
      screen.getByRole('toolbar', { name: 'List controls' })
    ).toBeInTheDocument()
  })

  it('offers the time range control', () => {
    renderPanel({ segment: 'toolbar' })
    expect(
      screen.getByRole('button', { name: /^Change time range/ })
    ).toBeInTheDocument()
  })

  it('names the sort control after the current field and direction', () => {
    renderPanel({ segment: 'toolbar' })
    expect(
      screen.getByRole('button', { name: 'Sort by Duration, descending' })
    ).toBeInTheDocument()
  })

  it('names the sort control ascending when sorted ascending', () => {
    renderPanel({
      segment: 'toolbar',
      sortValue: 'time',
      sortDirection: 'asc',
    })
    expect(
      screen.getByRole('button', { name: 'Sort by Start time, ascending' })
    ).toBeInTheDocument()
  })

  it('reports the sort menu as closed until it is opened', () => {
    renderPanel({ segment: 'toolbar' })
    expect(
      screen.getByRole('button', { name: 'Sort by Duration, descending' })
    ).toHaveAttribute('aria-expanded', 'false')
    expect(
      screen.queryByRole('menu', { name: 'Sort by' })
    ).not.toBeInTheDocument()
  })

  it('opens the sort menu from the trigger and reports it expanded', async () => {
    renderPanel({ segment: 'toolbar' })
    const menu = await openSortMenu()
    expect(menu).toBeVisible()
    await waitFor(() =>
      expect(screen.getByRole('button', { name: /^Sort by/ })).toHaveAttribute(
        'aria-expanded',
        'true'
      )
    )
  })

  it('focuses the selected option when the trigger opens the menu', async () => {
    renderPanel({ segment: 'toolbar' })
    await openSortMenu()
    expect(
      screen.getByRole('menuitemradio', { name: 'Duration' })
    ).toHaveFocus()
  })

  it('opens to the first or last option with ArrowDown or ArrowUp', async () => {
    renderPanel({ segment: 'toolbar' })
    const user = userEvent.setup()
    const trigger = screen.getByRole('button', { name: /^Sort by/ })
    trigger.focus()

    await user.keyboard('{ArrowDown}')
    expect(
      screen.getByRole('menuitemradio', { name: 'Start time' })
    ).toHaveFocus()
    await user.keyboard('{Escape}')
    await user.keyboard('{ArrowUp}')
    expect(
      screen.getByRole('menuitemradio', { name: 'Span count' })
    ).toHaveFocus()
  })

  it('lists every sort option in the open sort menu', async () => {
    renderPanel({ segment: 'toolbar' })
    await openSortMenu()
    for (const option of sortOptions) {
      expect(
        screen.getByRole('menuitemradio', { name: option.label })
      ).toBeInTheDocument()
    }
  })

  it('checks only the sort option currently in use', async () => {
    renderPanel({ segment: 'toolbar' })
    await openSortMenu()
    expect(
      screen.getByRole('menuitemradio', { name: 'Duration' })
    ).toHaveAttribute('aria-checked', 'true')
    expect(
      screen.getByRole('menuitemradio', { name: 'Start time' })
    ).toHaveAttribute('aria-checked', 'false')
  })

  it('moves focus with arrows and Home/End, wrapping at the menu edges', async () => {
    renderPanel({ segment: 'toolbar' })
    const user = userEvent.setup()
    await openSortMenu()

    await user.keyboard('{ArrowDown}')
    expect(
      screen.getByRole('menuitemradio', { name: 'Span count' })
    ).toHaveFocus()
    await user.keyboard('{ArrowDown}')
    expect(
      screen.getByRole('menuitemradio', { name: 'Start time' })
    ).toHaveFocus()
    await user.keyboard('{End}')
    expect(
      screen.getByRole('menuitemradio', { name: 'Span count' })
    ).toHaveFocus()
    await user.keyboard('{Home}')
    expect(
      screen.getByRole('menuitemradio', { name: 'Start time' })
    ).toHaveFocus()
  })

  it('closes on Escape and restores focus to the trigger', async () => {
    renderPanel({ segment: 'toolbar' })
    const user = userEvent.setup()
    await openSortMenu()
    await user.keyboard('{Escape}')

    expect(
      screen.queryByRole('menu', { name: 'Sort by' })
    ).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: /^Sort by/ })).toHaveFocus()
  })

  it('requests descending order when a magnitude field is chosen first', async () => {
    const onSortChange = vi.fn()
    renderPanel({ segment: 'toolbar', onSortChange })
    await openSortMenu()
    await userEvent.click(
      screen.getByRole('menuitemradio', { name: 'Span count' })
    )
    expect(onSortChange).toHaveBeenCalledWith('spanCount', 'desc')
  })

  it('requests ascending order when a different field is chosen', async () => {
    const onSortChange = vi.fn()
    renderPanel({ segment: 'toolbar', onSortChange })
    await openSortMenu()
    await userEvent.click(
      screen.getByRole('menuitemradio', { name: 'Start time' })
    )
    expect(onSortChange).toHaveBeenCalledWith('time', 'asc')
  })

  it('flips a descending field to ascending when it is chosen again', async () => {
    const onSortChange = vi.fn()
    renderPanel({ segment: 'toolbar', onSortChange })
    await openSortMenu()
    await userEvent.click(
      screen.getByRole('menuitemradio', { name: 'Duration' })
    )
    expect(onSortChange).toHaveBeenCalledWith('duration', 'asc')
  })

  it('flips an ascending field to descending when it is chosen again', async () => {
    const onSortChange = vi.fn()
    renderPanel({
      segment: 'toolbar',
      sortDirection: 'asc',
      onSortChange,
    })
    await openSortMenu()
    await userEvent.click(
      screen.getByRole('menuitemradio', { name: 'Duration' })
    )
    expect(onSortChange).toHaveBeenCalledWith('duration', 'desc')
  })

  it('closes the sort menu after a choice is made', async () => {
    renderPanel({ segment: 'toolbar' })
    await openSortMenu()
    await userEvent.click(
      screen.getByRole('menuitemradio', { name: 'Start time' })
    )
    await waitFor(() =>
      expect(screen.getByRole('button', { name: /^Sort by/ })).toHaveAttribute(
        'aria-expanded',
        'false'
      )
    )
    expect(
      screen.queryByRole('menu', { name: 'Sort by' })
    ).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: /^Sort by/ })).toHaveFocus()
  })

  it('leaves out the query editor', () => {
    renderPanel({ segment: 'toolbar' })
    expect(
      screen.queryByRole('button', { name: 'Clear search' })
    ).not.toBeInTheDocument()
  })
})

describe('DrawerSearchPanel search segment', () => {
  it('renders the query editor', () => {
    renderPanel({ segment: 'search' })
    expect(screen.getByRole('textbox')).toBeInTheDocument()
  })

  it('offers help and clear controls for the query', () => {
    renderPanel({ segment: 'search' })
    expect(
      screen.getByRole('button', { name: 'Search query help' })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Clear search' })
    ).toBeInTheDocument()
  })

  it('hands the caller submit and clear handles once the editor is ready', () => {
    const onSearchReady = vi.fn()
    renderPanel({ segment: 'search', onSearchReady })
    expect(onSearchReady).toHaveBeenCalledTimes(1)
    const api = onSearchReady.mock.calls[0][0]
    expect(api.submit).toBeTypeOf('function')
    expect(api.clear).toBeTypeOf('function')
  })

  it('leaves out the list controls toolbar', () => {
    renderPanel({ segment: 'search' })
    expect(
      screen.queryByRole('toolbar', { name: 'List controls' })
    ).not.toBeInTheDocument()
  })
})

describe('DrawerSearchPanel full segment', () => {
  it('renders both the list controls and the query editor by default', () => {
    renderPanel()
    expect(
      screen.getByRole('toolbar', { name: 'List controls' })
    ).toBeInTheDocument()
    expect(screen.getByRole('textbox')).toBeInTheDocument()
  })
})
