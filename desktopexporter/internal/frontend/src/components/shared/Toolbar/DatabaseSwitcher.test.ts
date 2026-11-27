// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import DatabaseSwitcher from './DatabaseSwitcher.svelte'
import { telemetryAPI } from '@/services/telemetry-service'
import { renderWithContexts } from '@/test/render-helpers'
import type { DatabaseList } from '@/types/api-types'

const managedInitial: DatabaseList = {
  managed: true,
  active: 'current.duckdb',
  directory: '/Users/test/Library/Application Support/otel-desktop-viewer',
  databases: [
    {
      name: 'current.duckdb',
      sizeBytes: 12288,
      modifiedAt: '2026-11-26T18:00:00Z',
      active: true,
    },
    {
      name: 'checkout-incident.duckdb',
      sizeBytes: 524288,
      modifiedAt: '2026-11-26T17:00:00Z',
      active: false,
    },
  ],
}

afterEach(() => vi.restoreAllMocks())

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

  it('lists managed sessions and switches when another session is selected', async () => {
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

    const second = await screen.findByRole('button', {
      name: /checkout-incident/,
    })
    await user.click(second)

    expect(openSpy).toHaveBeenCalledWith('checkout-incident.duckdb')
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
            sizeBytes: 4096,
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
    expect(newSpy).toHaveBeenLastCalledWith('after-dinner')
    await screen.findByRole('button', { name: 'Session: after-dinner' })
  })
})
