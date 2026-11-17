// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import ValueCopyButton from './ValueCopyButton.svelte'
import { copyText } from '@/utils/clipboard'

vi.mock('@/utils/clipboard', () => ({ copyText: vi.fn() }))

beforeEach(() => {
  vi.mocked(copyText).mockReset().mockResolvedValue()
})

describe('ValueCopyButton', () => {
  it('still copies an immediate value without changing its text', async () => {
    render(ValueCopyButton, { text: '{"value":-0.0}', label: 'value' })
    await userEvent.click(screen.getByRole('button', { name: 'Copy value' }))
    expect(copyText).toHaveBeenCalledWith('{"value":-0.0}')
    expect(screen.getByRole('status')).toHaveTextContent('Copied value.')
  })

  it('loads only on activation and ignores repeated activation while pending', async () => {
    let finish!: (text: string) => void
    const loadText = vi.fn(
      () =>
        new Promise<string>(resolve => {
          finish = resolve
        })
    )
    render(ValueCopyButton, { loadText, label: 'metric' })
    expect(loadText).not.toHaveBeenCalled()
    const button = screen.getByRole('button', { name: 'Copy metric' })
    await userEvent.click(button)
    await userEvent.click(button)
    expect(loadText).toHaveBeenCalledTimes(1)
    expect(button).toHaveAttribute('aria-busy', 'true')
    expect(copyText).not.toHaveBeenCalled()
    finish('{"value":-0.0,"count":18446744073709551615}\n')
    await waitFor(() =>
      expect(copyText).toHaveBeenCalledWith(
        '{"value":-0.0,"count":18446744073709551615}\n'
      )
    )
    expect(button).toHaveAttribute('aria-busy', 'false')
    expect(screen.getByRole('status')).toHaveTextContent('Copied metric.')
  })

  it('reports load errors without copying and permits retry', async () => {
    const loadText = vi
      .fn()
      .mockRejectedValueOnce(new Error('not found'))
      .mockResolvedValueOnce('{}')
    render(ValueCopyButton, { loadText, label: 'metric' })
    const button = screen.getByRole('button', { name: 'Copy metric' })
    await userEvent.click(button)
    expect(copyText).not.toHaveBeenCalled()
    expect(screen.getByRole('status')).toHaveTextContent(
      'Could not copy metric.'
    )
    await userEvent.click(button)
    expect(copyText).toHaveBeenCalledWith('{}')
    expect(screen.getByRole('status')).toHaveTextContent('Copied metric.')
  })

  it('aborts a stale load when the selected source changes', async () => {
    let finish!: (text: string) => void
    const loadText = vi.fn(
      (_signal: AbortSignal) =>
        new Promise<string>(resolve => {
          finish = resolve
        })
    )
    const view = render(ValueCopyButton, { loadText, label: 'metric' })
    await userEvent.click(screen.getByRole('button', { name: 'Copy metric' }))
    const signal = loadText.mock.calls[0]![0]
    const nextLoad = vi.fn().mockResolvedValue('new metric')
    await view.rerender({ loadText: nextLoad, label: 'metric' })
    expect(signal.aborted).toBe(true)
    finish('old metric')
    await userEvent.click(screen.getByRole('button', { name: 'Copy metric' }))
    expect(copyText).toHaveBeenCalledExactlyOnceWith('new metric')
  })

  it('aborts an outstanding load on unmount', async () => {
    let finish!: (text: string) => void
    const loadText = vi.fn(
      (_signal: AbortSignal) =>
        new Promise<string>(resolve => {
          finish = resolve
        })
    )
    const view = render(ValueCopyButton, { loadText, label: 'metric' })
    await userEvent.click(screen.getByRole('button', { name: 'Copy metric' }))
    const signal = loadText.mock.calls[0]![0]
    view.unmount()
    expect(signal.aborted).toBe(true)
    finish('stale metric')
    await Promise.resolve()
    expect(copyText).not.toHaveBeenCalled()
  })
})
