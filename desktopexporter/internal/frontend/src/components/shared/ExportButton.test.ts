// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import ExportButton from './ExportButton.svelte'
import { downloadOTLP } from '@/services/export-service'

vi.mock('@/services/export-service', () => ({ downloadOTLP: vi.fn() }))

afterEach(() => vi.resetAllMocks())

describe('ExportButton', () => {
  it('offers both formats and supports keyboard selection and dismissal', async () => {
    vi.mocked(downloadOTLP).mockResolvedValue()
    render(ExportButton, { signal: 'trace', id: 'trace-id' })
    const trigger = screen.getByRole('button', { name: 'Export trace' })
    await userEvent.click(trigger)
    expect(screen.getByRole('menuitem', { name: 'OTLP JSON' })).toHaveFocus()
    await userEvent.keyboard('{ArrowDown}')
    expect(
      screen.getByRole('menuitem', { name: 'OTLP protobuf' })
    ).toHaveFocus()
    await userEvent.keyboard('{Enter}')
    await waitFor(() =>
      expect(downloadOTLP).toHaveBeenCalledWith(
        'trace',
        'trace-id',
        'protobuf',
        expect.any(AbortSignal)
      )
    )
    expect(trigger).toHaveAttribute('aria-expanded', 'false')
    await userEvent.click(trigger)
    await userEvent.keyboard('{Escape}')
    expect(trigger).toHaveFocus()
    expect(trigger).toHaveAttribute('aria-expanded', 'false')
  })

  it('reports failures and allows retrying', async () => {
    vi.mocked(downloadOTLP).mockRejectedValueOnce(
      new Error('export record not found')
    )
    render(ExportButton, { signal: 'log', id: 'log-id' })
    const trigger = screen.getByRole('button', { name: 'Export log' })
    await userEvent.click(trigger)
    await userEvent.click(screen.getByRole('menuitem', { name: 'OTLP JSON' }))
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'export record not found'
    )
    expect(trigger).toBeEnabled()
    vi.mocked(downloadOTLP).mockResolvedValueOnce()
    await userEvent.click(trigger)
    await userEvent.click(screen.getByRole('menuitem', { name: 'OTLP JSON' }))
    await waitFor(() => expect(screen.queryByRole('alert')).toBeNull())
  })

  it('disables duplicate exports and aborts its request on unmount', async () => {
    vi.mocked(downloadOTLP).mockImplementation(
      (_signal, _id, _format, abort) =>
        new Promise((_resolve, reject) => {
          abort.addEventListener('abort', () => reject(abort.reason), {
            once: true,
          })
        })
    )
    const view = render(ExportButton, { signal: 'metric', id: 'metric-id' })
    const trigger = screen.getByRole('button', { name: 'Export metric' })
    await userEvent.click(trigger)
    await userEvent.click(screen.getByRole('menuitem', { name: 'OTLP JSON' }))
    expect(trigger).toHaveAttribute('aria-disabled', 'true')
    expect(trigger).toBeEnabled()
    expect(trigger).not.toHaveAttribute('popovertarget')
    const signal = vi.mocked(downloadOTLP).mock.calls[0]![3]
    expect(signal.aborted).toBe(false)
    view.unmount()
    expect(signal.aborted).toBe(true)
  })
})
