// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/svelte'
import { navigate } from '@/route'
import { setTestUrl } from '@/test/render-helpers'

vi.mock('@/pages/HomePage.svelte', () => import('@/test/RouteProbe.svelte'))
vi.mock('@/pages/TracesPage.svelte', () => import('@/test/RouteProbe.svelte'))
vi.mock('@/pages/MetricsPage.svelte', () => import('@/test/RouteProbe.svelte'))
vi.mock('@/pages/LogsPage.svelte', () => import('@/test/TimeProbe.svelte'))

import App from './App.svelte'

describe('App route focus recovery', () => {
  it('moves focus to main after a top-level SPA page change', async () => {
    setTestUrl('/')
    render(App)
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
    render(App)
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
    render(App)
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
    render(App)
    const main = screen.getByRole('main')
    const routeOutput = screen.getByTestId('route-path')
    const trigger = document.createElement('button')
    routeOutput.appendChild(trigger)
    trigger.focus()

    navigate('/logs')
    await waitFor(() => expect(trigger).not.toBeInTheDocument())
    await waitFor(() => expect(main).toHaveFocus())
  })
})
