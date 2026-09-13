// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { render, screen, waitFor } from '@testing-library/svelte'
import { navigate } from '@/route'
import { setTestUrl } from '@/test/render-helpers'
import RouteProbe from '@/test/RouteProbe.svelte'
import TimeProbe from '@/test/TimeProbe.svelte'
import App, { type AppPages } from './App.svelte'

const probePages = {
  home: RouteProbe,
  traces: RouteProbe,
  metrics: RouteProbe,
  logs: TimeProbe,
} satisfies AppPages

describe('App route selection', () => {
  it.each([
    ['/', 'home'],
    ['/other', 'home'],
    ['/traces', 'traces'],
    ['/traces/abc', 'traces'],
    ['/traces-other', 'home'],
    ['/metrics', 'metrics'],
    ['/metrics/abc', 'metrics'],
    ['/logs', 'logs'],
    ['/logs/abc', 'logs'],
  ] as const)('selects %s as the %s page', (path, selectedPage) => {
    setTestUrl(path)
    const pages = {
      home: selectedPage === 'home' ? TimeProbe : RouteProbe,
      traces: selectedPage === 'traces' ? TimeProbe : RouteProbe,
      metrics: selectedPage === 'metrics' ? TimeProbe : RouteProbe,
      logs: selectedPage === 'logs' ? TimeProbe : RouteProbe,
    } satisfies AppPages

    render(App, { pages })

    expect(screen.getByTestId('selection-type')).toBeInTheDocument()
    expect(screen.queryByTestId('route-path')).not.toBeInTheDocument()
  })
})

describe('App route focus recovery', () => {
  it('moves focus to main after a top-level SPA page change', async () => {
    setTestUrl('/')
    render(App, { pages: probePages })
    const main = screen.getByRole('main')
    const outside = document.createElement('button')
    document.body.appendChild(outside)
    outside.focus()

    navigate('/metrics')
    await waitFor(() => expect(main).toHaveFocus())
    outside.remove()
  })

  it('preserves focus across repeated master-detail pathname changes', async () => {
    setTestUrl('/metrics/a')
    render(App, { pages: probePages })
    const control = document.createElement('button')
    document.body.appendChild(control)
    control.focus()

    for (const path of ['/metrics/b', '/metrics/c']) {
      navigate(path)
      await waitFor(() =>
        expect(screen.getByTestId('route-path')).toHaveTextContent(path)
      )
      expect(control).toHaveFocus()
    }
    control.remove()
  })

  it('does not move focus for a query-only update', async () => {
    setTestUrl('/metrics')
    render(App, { pages: probePages })
    const outside = document.createElement('button')
    document.body.appendChild(outside)
    outside.focus()

    navigate('/metrics?start=10&end=20', 'replace')
    await waitFor(() =>
      expect(screen.getByTestId('route-query')).toHaveTextContent(
        '"start":"10","end":"20"'
      )
    )
    expect(outside).toHaveFocus()
    outside.remove()
  })

  it('recovers focus when a cross-page trigger is destroyed', async () => {
    setTestUrl('/metrics/a')
    render(App, { pages: probePages })
    const main = screen.getByRole('main')
    const routeOutput = screen.getByTestId('route-path')
    const trigger = document.createElement('button')
    routeOutput.appendChild(trigger)
    trigger.focus()
    let oldPageWasDestroyedBeforeFocus: boolean | undefined
    main.addEventListener(
      'focus',
      () => {
        oldPageWasDestroyedBeforeFocus = !document.body.contains(trigger)
      },
      { once: true }
    )

    navigate('/logs')
    await waitFor(() => expect(trigger).not.toBeInTheDocument())
    await waitFor(() => expect(main).toHaveFocus())
    expect(oldPageWasDestroyedBeforeFocus).toBe(true)
  })
})
