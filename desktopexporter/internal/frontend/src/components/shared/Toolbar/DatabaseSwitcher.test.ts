// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import DatabaseSwitcher, { formatBytes } from './DatabaseSwitcher.svelte'
import { telemetryAPI } from '@/services/telemetry-service'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'
import type { DatabaseList } from '@/types/api-types'

const managedInitial: DatabaseList = {
  managed: true,
  active: 'current.duckdb',
  databases: [
    {
      name: 'current.duckdb',
      sizeBytes: 12000,
      modifiedAt: '2026-11-26T18:00:00Z',
      active: true,
    },
    {
      name: 'checkout-incident.duckdb',
      sizeBytes: 500000,
      modifiedAt: '2026-11-26T17:00:00Z',
      active: false,
    },
  ],
}

afterEach(() => {
  vi.restoreAllMocks()
  setTestUrl('/')
})

describe('formatBytes', () => {
  it('uses 1000-based decimal units matching Home storage display', () => {
    expect(formatBytes(0)).toBe('0 B')
    expect(formatBytes(512)).toBe('512 B')
    expect(formatBytes(1000)).toBe('1.0 KB')
    expect(formatBytes(12000)).toBe('12 KB')
    expect(formatBytes(1_500_000)).toBe('1.5 MB')
    expect(formatBytes(2_000_000_000)).toBe('2.0 GB')
  })

  it('handles non-finite, negative, and boundary rounding values cleanly', () => {
    expect(formatBytes(Number.NaN)).toBe('0 B')
    expect(formatBytes(Number.POSITIVE_INFINITY)).toBe('0 B')
    expect(formatBytes(-42)).toBe('0 B')
    expect(formatBytes(999.6)).toBe('1.0 KB')
    expect(formatBytes(9950)).toBe('10 KB')
    expect(formatBytes(999_500)).toBe('1.0 MB')
    expect(formatBytes(1_048_525)).toBe('1.0 MB')
  })
})

describe('DatabaseSwitcher', () => {
  it('stays hidden when storage is not managed', async () => {
    vi.spyOn(telemetryAPI, 'listDatabases').mockResolvedValue({
      managed: false,
      active: '',
      databases: [],
    })
    renderWithContexts(DatabaseSwitcher)
    await waitFor(() => expect(telemetryAPI.listDatabases).toHaveBeenCalled())
    expect(
      screen.queryByRole('button', { name: /^Session:/ })
    ).not.toBeInTheDocument()
  })

  it('lists managed sessions, focuses the dialog input on open, and switches sessions', async () => {
    const user = userEvent.setup()
    vi.spyOn(telemetryAPI, 'listDatabases').mockResolvedValue(managedInitial)
    const openSpy = vi.spyOn(telemetryAPI, 'openDatabase').mockResolvedValue({
      ...managedInitial,
      active: 'checkout-incident.duckdb',
      databases: [
        { ...managedInitial.databases[1]!, active: true },
        { ...managedInitial.databases[0]!, active: false },
      ],
    })

    renderWithContexts(DatabaseSwitcher)
    const trigger = await screen.findByRole('button', {
      name: 'Session: current',
    })
    await user.click(trigger)

    expect(
      screen.getByRole('textbox', { name: 'New session name' })
    ).toHaveFocus()

    const second = await screen.findByRole('button', {
      name: /checkout-incident/,
    })
    expect(second).toHaveTextContent('500 KB')
    await user.click(second)

    expect(openSpy).toHaveBeenCalledWith(
      'checkout-incident.duckdb',
      expect.any(AbortSignal)
    )
    await screen.findByRole('button', {
      name: 'Session: checkout-incident',
    })
  })

  it('creates a new session from the popover form and surfaces server errors', async () => {
    const user = userEvent.setup()
    vi.spyOn(telemetryAPI, 'listDatabases').mockResolvedValue(managedInitial)
    const newSpy = vi
      .spyOn(telemetryAPI, 'newDatabase')
      .mockRejectedValueOnce(new Error('Database already exists'))
      .mockResolvedValueOnce({
        ...managedInitial,
        active: 'after-dinner.duckdb',
        databases: [
          {
            name: 'after-dinner.duckdb',
            sizeBytes: 4000,
            modifiedAt: '2026-11-26T18:10:00Z',
            active: true,
          },
          { ...managedInitial.databases[0]!, active: false },
          managedInitial.databases[1]!,
        ],
      })

    renderWithContexts(DatabaseSwitcher)
    await user.click(
      await screen.findByRole('button', { name: 'Session: current' })
    )

    const input = screen.getByRole('textbox', { name: 'New session name' })
    const submit = screen.getByRole('button', { name: 'New' })

    await user.type(input, 'current')
    await user.click(submit)
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Database already exists'
    )

    await user.clear(input)
    await user.type(input, 'after-dinner')
    await user.click(submit)
    expect(newSpy).toHaveBeenLastCalledWith(
      'after-dinner',
      expect.any(AbortSignal)
    )
    await screen.findByRole('button', { name: 'Session: after-dinner' })
  })

  it('surfaces non-abort listDatabases errors when reopening the popover', async () => {
    const user = userEvent.setup()
    vi.spyOn(telemetryAPI, 'listDatabases')
      .mockResolvedValueOnce(managedInitial)
      .mockRejectedValueOnce(new Error('Session directory unreadable'))

    renderWithContexts(DatabaseSwitcher)
    await user.click(
      await screen.findByRole('button', { name: 'Session: current' })
    )

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Session directory unreadable'
    )
  })

  it('aborts an in-flight switch request when unmounted', async () => {
    const user = userEvent.setup()
    vi.spyOn(telemetryAPI, 'listDatabases').mockResolvedValue(managedInitial)
    let capturedSignal: AbortSignal | undefined
    vi.spyOn(telemetryAPI, 'openDatabase').mockImplementation(
      (_name, signal) => {
        capturedSignal = signal
        return new Promise(() => {})
      }
    )

    const { unmount } = renderWithContexts(DatabaseSwitcher)
    await user.click(
      await screen.findByRole('button', { name: 'Session: current' })
    )
    await user.click(
      await screen.findByRole('button', { name: /checkout-incident/ })
    )

    expect(capturedSignal).toBeDefined()
    expect(capturedSignal?.aborted).toBe(false)
    unmount()
    expect(capturedSignal?.aborted).toBe(true)
  })

  it.each([
    [
      '/traces/trace-1?start=10&end=20&q=err&span=s1&event=0&log=l1&focus=1',
      '/traces?start=10&end=20&q=err',
    ],
    ['/traces?q=err&span=s1&series=x', '/traces?q=err'],
    [
      '/logs/log-1?time=all&q=severity%3DERROR&log=l1',
      '/logs?time=all&q=severity%3DERROR',
    ],
    [
      '/metrics/metric-1?start=10&end=20&series=s1&agg=rate&dp=d1&visible=s1',
      '/metrics?start=10&end=20',
    ],
    [
      '/evals?experiment=e1&result=r1&row=experiment/e1&esort=name',
      '/evals?esort=name',
    ],
    ['/?q=checkout&time=all&span=s1', '/?q=checkout&time=all'],
  ])('normalizes %s to %s on switch', async (initialUrl, expectedUrl) => {
    const user = userEvent.setup()
    setTestUrl(initialUrl)
    vi.spyOn(telemetryAPI, 'listDatabases').mockResolvedValue(managedInitial)
    vi.spyOn(telemetryAPI, 'openDatabase').mockResolvedValue({
      ...managedInitial,
      active: 'checkout-incident.duckdb',
      databases: [
        { ...managedInitial.databases[1]!, active: true },
        { ...managedInitial.databases[0]!, active: false },
      ],
    })

    renderWithContexts(DatabaseSwitcher)
    await user.click(
      await screen.findByRole('button', { name: 'Session: current' })
    )
    await user.click(
      await screen.findByRole('button', { name: /checkout-incident/ })
    )

    await waitFor(() =>
      expect(window.location.pathname + window.location.search).toBe(
        expectedUrl
      )
    )
  })
})
