// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { tick } from 'svelte'
import { screen, waitFor } from '@testing-library/svelte'
import { navigate } from '@/route'
import RouteProbe from '@/test/RouteProbe.svelte'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'

function reportedPath(): string {
  return screen.getByTestId('route-path').textContent ?? ''
}

function reportedQuery(): Record<string, string> {
  return JSON.parse(screen.getByTestId('route-query').textContent ?? '{}')
}

describe('route context', () => {
  it('reports the URL that was live when it was created', () => {
    setTestUrl('/traces/abc?span=3')
    renderWithContexts(RouteProbe)
    expect(reportedPath()).toBe('/traces/abc')
    expect(reportedQuery()).toEqual({ span: '3' })
  })

  it('reports the new path after a navigation', async () => {
    setTestUrl('/traces')
    renderWithContexts(RouteProbe)
    navigate('/metrics')
    await tick()
    expect(reportedPath()).toBe('/metrics')
  })

  it('reports the new query after a navigation', async () => {
    setTestUrl('/traces')
    renderWithContexts(RouteProbe)
    navigate('/traces?start=1&end=2')
    await tick()
    expect(reportedQuery()).toEqual({ start: '1', end: '2' })
  })

  it('drops query params that the navigation left behind', async () => {
    setTestUrl('/traces?span=3')
    renderWithContexts(RouteProbe)
    navigate('/traces')
    await tick()
    expect(reportedQuery()).toEqual({})
  })
})

describe('route context history semantics', () => {
  it('adds a history entry when navigating in push mode', () => {
    setTestUrl('/traces')
    renderWithContexts(RouteProbe)
    const before = window.history.length
    navigate('/metrics', 'push')
    expect(window.history.length).toBe(before + 1)
  })

  it('defaults to push mode', () => {
    setTestUrl('/traces')
    renderWithContexts(RouteProbe)
    const before = window.history.length
    navigate('/metrics')
    expect(window.history.length).toBe(before + 1)
  })

  it('does not add a history entry when navigating in replace mode', () => {
    setTestUrl('/traces')
    renderWithContexts(RouteProbe)
    const before = window.history.length
    navigate('/metrics', 'replace')
    expect(window.history.length).toBe(before)
  })

  it('reports the new path in replace mode too', async () => {
    setTestUrl('/traces')
    renderWithContexts(RouteProbe)
    navigate('/logs', 'replace')
    await tick()
    expect(reportedPath()).toBe('/logs')
  })
})

describe('route context popstate handling', () => {
  it('ignores a URL change made behind its back', async () => {
    setTestUrl('/traces')
    renderWithContexts(RouteProbe)
    window.history.replaceState(null, '', '/metrics')
    await tick()
    expect(reportedPath()).toBe('/traces')
  })

  it('re-reads the URL when a popstate event fires', async () => {
    setTestUrl('/traces')
    renderWithContexts(RouteProbe)
    navigate('/metrics?agg=rate')
    await tick()

    window.history.replaceState(null, '', '/traces?span=3')
    window.dispatchEvent(new PopStateEvent('popstate'))
    await tick()

    expect(reportedPath()).toBe('/traces')
    expect(reportedQuery()).toEqual({ span: '3' })
  })

  it('keeps following popstate events after several navigations', async () => {
    setTestUrl('/traces')
    renderWithContexts(RouteProbe)
    navigate('/metrics')
    navigate('/logs')
    await tick()
    expect(reportedPath()).toBe('/logs')

    window.history.replaceState(null, '', '/metrics')
    window.dispatchEvent(new PopStateEvent('popstate'))
    await tick()
    expect(reportedPath()).toBe('/metrics')
  })

  // jsdom applies history.back() on a later task rather than synchronously,
  // so the assertion has to wait for the popstate it eventually fires.
  it('returns to the previous pushed URL when the browser goes back', async () => {
    setTestUrl('/traces?span=3')
    renderWithContexts(RouteProbe)
    navigate('/metrics')
    await tick()
    expect(reportedPath()).toBe('/metrics')

    window.history.back()
    await waitFor(() => {
      expect(reportedPath()).toBe('/traces')
    })
    expect(reportedQuery()).toEqual({ span: '3' })
  })

  it('stays on the replaced URL when the browser goes back', async () => {
    setTestUrl('/traces')
    renderWithContexts(RouteProbe)
    navigate('/metrics')
    navigate('/logs', 'replace')
    await tick()
    expect(reportedPath()).toBe('/logs')

    window.history.back()
    await waitFor(() => {
      expect(reportedPath()).toBe('/traces')
    })
  })
})
