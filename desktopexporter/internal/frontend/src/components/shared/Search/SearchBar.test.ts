// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { createRawSnippet } from 'svelte'
import { screen, within } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import SearchBar from './SearchBar.svelte'
import { renderWithContexts } from '@/test/render-helpers'

const search = createRawSnippet(() => ({
  render: () => '<input type="search" aria-label="Search fixture" />',
}))

describe('SearchBar', () => {
  it('keeps the search field visible without a collapse control', () => {
    renderWithContexts(SearchBar, { search })
    const bar = screen.getByRole('search', { name: 'Workspace search' })
    const refresh = screen.getByRole('button', { name: 'Refresh' })
    const field = screen.getByRole('searchbox', { name: 'Search fixture' })
    const time = screen.getByRole('button', { name: /^Change time range/ })

    expect(bar).toContainElement(refresh)
    expect(bar).toContainElement(field)
    expect(bar).toContainElement(time)
    expect(
      screen.queryByRole('combobox', { name: 'Result limit' })
    ).not.toBeInTheDocument()
    expect(time.textContent?.trim()).toBe('')
    expect(refresh).toHaveClass('text-base-content')
    expect(time).toHaveClass('text-base-content')
    expect(within(bar).getAllByRole('button')).toHaveLength(2)
    expect(
      screen.queryByRole('button', { name: /Expand search|Collapse search/ })
    ).not.toBeInTheDocument()
    expect(field).toBeVisible()
  })

  it('calls the supplied refresh handler', async () => {
    const onRefresh = vi.fn()
    renderWithContexts(SearchBar, { search, onRefresh })
    await userEvent.click(screen.getByRole('button', { name: 'Refresh' }))
    expect(onRefresh).toHaveBeenCalledTimes(1)
  })

  it('preserves the refresh status and its accessible name', () => {
    renderWithContexts(SearchBar, {
      search,
      refreshPulse: true,
      refreshAsideTip: 'New traces',
    })
    expect(
      screen.getByRole('button', { name: 'Refresh: New traces' })
    ).toBeInTheDocument()
    expect(screen.getByRole('status')).toHaveTextContent('New traces')
  })
})
