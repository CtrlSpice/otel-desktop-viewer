// @vitest-environment jsdom
import { beforeAll, describe, expect, it, vi } from 'vitest'
import { screen } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import DrawerSearchPanel from './DrawerSearchPanel.svelte'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'

const sortOptions = [
  { value: 'time', label: 'Start time' },
  { value: 'duration', label: 'Duration' },
]

// jsdom ships the popover UA styles but not the popover JS API, so the sort
// menu can never be opened here: it stays in the DOM, hidden. Tests reach it
// with `hidden: true`, and this stand-in keeps the component's close call from
// throwing when an option is chosen.
beforeAll(() => {
  if (typeof HTMLElement.prototype.hidePopover !== 'function') {
    HTMLElement.prototype.hidePopover = () => {}
  }
})

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
  })

  it('lists every sort option in a sort menu', () => {
    renderPanel({ segment: 'toolbar' })
    const menu = screen.getByRole('menu', { name: 'Sort by', hidden: true })
    expect(menu).toBeInTheDocument()
    for (const option of sortOptions) {
      expect(
        screen.getByRole('menuitemradio', {
          name: option.label,
          hidden: true,
        })
      ).toBeInTheDocument()
    }
  })

  it('checks only the sort option currently in use', () => {
    renderPanel({ segment: 'toolbar' })
    expect(
      screen.getByRole('menuitemradio', { name: 'Duration', hidden: true })
    ).toHaveAttribute('aria-checked', 'true')
    expect(
      screen.getByRole('menuitemradio', { name: 'Start time', hidden: true })
    ).toHaveAttribute('aria-checked', 'false')
  })

  it('requests ascending order when a different field is chosen', async () => {
    const onSortChange = vi.fn()
    renderPanel({ segment: 'toolbar', onSortChange })
    await userEvent.click(
      screen.getByRole('menuitemradio', { name: 'Start time', hidden: true })
    )
    expect(onSortChange).toHaveBeenCalledWith('time', 'asc')
  })

  it('flips a descending field to ascending when it is chosen again', async () => {
    const onSortChange = vi.fn()
    renderPanel({ segment: 'toolbar', onSortChange })
    await userEvent.click(
      screen.getByRole('menuitemradio', { name: 'Duration', hidden: true })
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
    await userEvent.click(
      screen.getByRole('menuitemradio', { name: 'Duration', hidden: true })
    )
    expect(onSortChange).toHaveBeenCalledWith('duration', 'desc')
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
