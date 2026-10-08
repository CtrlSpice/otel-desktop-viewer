// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import ExportButton from './ExportButton.svelte'
import { downloadOTLP } from '@/services/export-service'

vi.mock('@/services/export-service', () => ({ downloadOTLP: vi.fn() }))

afterEach(() => vi.resetAllMocks())

describe('ExportButton', () => {
  it('does not export without a selected identifier', async () => {
    render(ExportButton, { signal: 'trace', id: '' })
    const trigger = screen.getByRole('button', { name: 'Export trace' })
    expect(trigger).toBeDisabled()
    await userEvent.click(trigger)
    expect(downloadOTLP).not.toHaveBeenCalled()
  })

  it('downloads JSON with one click and keeps keyboard focus', async () => {
    vi.mocked(downloadOTLP).mockResolvedValue()
    render(ExportButton, { signal: 'trace', id: 'trace-id' })
    const trigger = screen.getByRole('button', { name: 'Export trace' })
    await userEvent.click(trigger)
    await waitFor(() =>
      expect(downloadOTLP).toHaveBeenCalledWith(
        'trace',
        'trace-id',
        expect.any(AbortSignal)
      )
    )
    expect(trigger).toHaveFocus()
    expect(screen.queryByRole('menu')).toBeNull()
    await userEvent.keyboard('{Enter}')
    await waitFor(() => expect(downloadOTLP).toHaveBeenCalledTimes(2))
  })

  it('reports failures and allows retrying', async () => {
    vi.mocked(downloadOTLP).mockRejectedValueOnce(
      new Error('export record not found')
    )
    render(ExportButton, { signal: 'log', id: 'log-id' })
    const trigger = screen.getByRole('button', { name: 'Export log' })
    await userEvent.click(trigger)
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'export record not found'
    )
    expect(trigger).toBeEnabled()
    vi.mocked(downloadOTLP).mockResolvedValueOnce()
    await userEvent.click(trigger)
    await waitFor(() => expect(screen.queryByRole('alert')).toBeNull())
  })

  it('disables duplicate exports and aborts its request on unmount', async () => {
    vi.mocked(downloadOTLP).mockImplementation(
      (_signal, _id, abort) =>
        new Promise((_resolve, reject) => {
          abort.addEventListener('abort', () => reject(abort.reason), {
            once: true,
          })
        })
    )
    const view = render(ExportButton, { signal: 'metric', id: 'metric-id' })
    const trigger = screen.getByRole('button', { name: 'Export metric' })
    await userEvent.click(trigger)
    expect(trigger).toHaveAttribute('aria-disabled', 'true')
    expect(trigger).toBeEnabled()
    expect(trigger).not.toHaveAttribute('popovertarget')
    await userEvent.click(trigger)
    await userEvent.keyboard('{Enter}')
    expect(downloadOTLP).toHaveBeenCalledTimes(1)
    expect(trigger).toHaveFocus()
    const signal = vi.mocked(downloadOTLP).mock.calls[0]![2]
    expect(signal.aborted).toBe(false)
    view.unmount()
    expect(signal.aborted).toBe(true)
  })
})
