// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { createRawSnippet } from 'svelte'
import { screen, within } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import WorkspaceToolbar from './WorkspaceToolbar.svelte'
import { renderWithContexts } from '@/test/render-helpers'

const search = createRawSnippet(() => ({
  render: () => '<input type="search" aria-label="Search fixture" />',
}))

describe('WorkspaceToolbar', () => {
  it('keeps search visible with standalone refresh and time controls', () => {
    renderWithContexts(WorkspaceToolbar, { search })
    const bar = screen.getByRole('search', { name: 'Workspace search' })
    const refresh = screen.getByRole('button', { name: 'Refresh' })
    const field = screen.getByRole('searchbox', { name: 'Search fixture' })
    const time = screen.getByRole('button', { name: /^Change time range/ })

    const toolbar = screen.getByRole('toolbar', { name: 'Workspace controls' })
    expect(toolbar).toContainElement(refresh)
    expect(toolbar).toContainElement(time)
    expect(bar).not.toContainElement(refresh)
    expect(bar).toContainElement(field)
    expect(bar).not.toContainElement(time)
    expect(
      screen.queryByRole('combobox', { name: 'Result limit' })
    ).not.toBeInTheDocument()
    expect(time).toHaveAccessibleName('Change time range, All time')
    expect(refresh).toHaveClass('text-base-content')
    expect(time).toHaveClass('text-base-content')
    expect(within(bar).queryAllByRole('button')).toHaveLength(0)
    expect(
      screen.queryByRole('button', { name: /Expand search|Collapse search/ })
    ).not.toBeInTheDocument()
    expect(field).toBeVisible()
  })

  it('calls the supplied refresh handler', async () => {
    const onRefresh = vi.fn()
    renderWithContexts(WorkspaceToolbar, { search, onRefresh })
    await userEvent.click(screen.getByRole('button', { name: 'Refresh' }))
    expect(onRefresh).toHaveBeenCalledTimes(1)
  })

  it('preserves the refresh status and its accessible name', () => {
    renderWithContexts(WorkspaceToolbar, {
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
