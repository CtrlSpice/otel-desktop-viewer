// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { createRawSnippet } from 'svelte'
import { screen } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import SignalNavRail from './SignalNavRail.svelte'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'

const railContent = createRawSnippet(() => ({
  render: () => '<p data-testid="rail-content">page body</p>',
}))

function renderRail() {
  return renderWithContexts(SignalNavRail, { children: railContent })
}

describe('SignalNavRail', () => {
  it('renders the primary navigation rail', () => {
    setTestUrl('/traces')
    renderRail()
    expect(
      screen.getByRole('complementary', { name: 'Primary navigation' })
    ).toBeInTheDocument()
  })

  it('renders a collapsed tab for each signal', () => {
    setTestUrl('/traces')
    renderRail()
    for (const label of ['Traces', 'Metrics', 'Logs']) {
      expect(screen.getByRole('button', { name: label })).toBeInTheDocument()
    }
  })

  it('marks the tab matching the current URL as the current page', () => {
    setTestUrl('/traces/some-trace-id')
    renderRail()
    expect(screen.getByRole('button', { name: 'Traces' })).toHaveAttribute(
      'aria-current',
      'page'
    )
    expect(screen.getByRole('button', { name: 'Metrics' })).not.toHaveAttribute(
      'aria-current'
    )
  })

  it('navigates to the signal when its tab is clicked', async () => {
    setTestUrl('/traces')
    renderRail()
    await userEvent.click(screen.getByRole('button', { name: 'Metrics' }))
    expect(window.location.pathname).toBe('/metrics')
    expect(screen.getByRole('button', { name: 'Metrics' })).toHaveAttribute(
      'aria-current',
      'page'
    )
  })

  it('renders its children in the content area', () => {
    setTestUrl('/traces')
    renderRail()
    expect(screen.getByTestId('rail-content')).toHaveTextContent('page body')
  })
})
