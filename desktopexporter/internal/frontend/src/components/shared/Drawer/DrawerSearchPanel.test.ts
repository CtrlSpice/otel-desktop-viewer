// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { screen } from '@testing-library/svelte'
import DrawerSearchPanel from './DrawerSearchPanel.svelte'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'

function renderPanel(props: Record<string, unknown> = {}) {
  setTestUrl('/traces')
  return renderWithContexts(DrawerSearchPanel, {
    signal: 'traces',
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

  it('keeps an accessible name on the icon-only time range control', () => {
    renderPanel({ segment: 'toolbar' })
    expect(
      screen.getByRole('button', { name: /^Change time range/ })
    ).toBeInTheDocument()
    expect(
      screen.queryByText('Time range', { exact: true })
    ).not.toBeInTheDocument()
  })

  it('does not offer a sort button or menu', () => {
    renderPanel({ segment: 'toolbar' })
    expect(
      screen.queryByRole('button', { name: /^Sort by/ })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('menu', { name: 'Sort by' })
    ).not.toBeInTheDocument()
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

  it('offers clear and help controls for the query', () => {
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
