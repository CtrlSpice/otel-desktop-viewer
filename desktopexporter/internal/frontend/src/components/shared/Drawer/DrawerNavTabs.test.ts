// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { screen } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import DrawerNavTabs, { isNavItemActive } from './DrawerNavTabs.svelte'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'

function renderTabs() {
  return renderWithContexts(DrawerNavTabs, { collapsed: false })
}

describe('isNavItemActive', () => {
  it('treats home as active only on the root path', () => {
    expect(isNavItemActive('home', '/')).toBe(true)
    expect(isNavItemActive('home', '/traces')).toBe(false)
  })

  it('treats a signal as active on its list path', () => {
    expect(isNavItemActive('traces', '/traces')).toBe(true)
    expect(isNavItemActive('metrics', '/metrics')).toBe(true)
    expect(isNavItemActive('logs', '/logs')).toBe(true)
  })

  it('treats a signal as active on a nested item path', () => {
    expect(isNavItemActive('traces', '/traces/abc')).toBe(true)
    expect(isNavItemActive('metrics', '/metrics/some.metric')).toBe(true)
    expect(isNavItemActive('logs', '/logs/log-1')).toBe(true)
  })

  it('does not match a path that merely starts with the signal name', () => {
    expect(isNavItemActive('traces', '/tracesfoo')).toBe(false)
    expect(isNavItemActive('metrics', '/metricsfoo')).toBe(false)
    expect(isNavItemActive('logs', '/logsfoo')).toBe(false)
  })

  it('does not match a different signal', () => {
    expect(isNavItemActive('traces', '/metrics')).toBe(false)
    expect(isNavItemActive('logs', '/traces/abc')).toBe(false)
  })

  it('is never active for an unknown nav id', () => {
    for (const path of ['/', '/traces', '/metrics/abc', '/anything']) {
      expect(isNavItemActive('nope', path)).toBe(false)
    }
  })
})

describe('DrawerNavTabs (expanded)', () => {
  it('renders a labelled tab for each signal', () => {
    setTestUrl('/traces')
    renderTabs()
    for (const label of ['Traces', 'Metrics', 'Logs']) {
      expect(screen.getByRole('link', { name: label })).toBeInTheDocument()
      expect(screen.getByText(label)).toBeVisible()
    }
  })

  it('marks the tab matching the current URL as the current page', () => {
    setTestUrl('/metrics')
    renderTabs()
    expect(screen.getByRole('link', { name: 'Metrics' })).toHaveAttribute(
      'aria-current',
      'page'
    )
    expect(screen.getByRole('link', { name: 'Traces' })).not.toHaveAttribute(
      'aria-current'
    )
    expect(screen.getByRole('link', { name: 'Logs' })).not.toHaveAttribute(
      'aria-current'
    )
  })

  it('marks a signal tab as current while an item of that signal is open', () => {
    setTestUrl('/logs/log-42')
    renderTabs()
    expect(screen.getByRole('link', { name: 'Logs' })).toHaveAttribute(
      'aria-current',
      'page'
    )
  })

  it('marks no tab as current on a path outside the signals', () => {
    setTestUrl('/')
    renderTabs()
    for (const label of ['Traces', 'Metrics', 'Logs']) {
      expect(screen.getByRole('link', { name: label })).not.toHaveAttribute(
        'aria-current'
      )
    }
  })

  it('navigates to the signal list when its tab is clicked', async () => {
    setTestUrl('/traces')
    renderTabs()
    await userEvent.click(screen.getByRole('link', { name: 'Logs' }))
    expect(window.location.pathname).toBe('/logs')
  })

  it('moves the current-page marker to the tab that was clicked', async () => {
    setTestUrl('/traces')
    renderTabs()
    await userEvent.click(screen.getByRole('link', { name: 'Metrics' }))
    expect(screen.getByRole('link', { name: 'Metrics' })).toHaveAttribute(
      'aria-current',
      'page'
    )
    expect(screen.getByRole('link', { name: 'Traces' })).not.toHaveAttribute(
      'aria-current'
    )
  })

  it('leaves the trace item path for the trace list when Traces is clicked', async () => {
    setTestUrl('/traces/abc')
    renderTabs()
    await userEvent.click(screen.getByRole('link', { name: 'Traces' }))
    expect(window.location.pathname).toBe('/traces')
  })

  it('preserves the active time window in each real href', () => {
    setTestUrl('/traces?start=10&end=20&span=old')
    renderTabs()
    expect(screen.getByRole('link', { name: 'Metrics' })).toHaveAttribute(
      'href',
      '/metrics?start=10&end=20'
    )
  })

  it('leaves modified clicks to native anchor handling', () => {
    setTestUrl('/traces')
    renderTabs()
    const link = screen.getByRole('link', { name: 'Logs' })
    const event = new MouseEvent('click', {
      bubbles: true,
      cancelable: true,
      ctrlKey: true,
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
    expect(window.location.pathname).toBe('/traces')
  })
})
