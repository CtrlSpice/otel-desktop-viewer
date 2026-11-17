// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, cleanup, render, screen } from '@testing-library/svelte'
import LoadingStatus from './LoadingStatus.svelte'

describe('shared loading feedback', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => {
    cleanup()
    vi.useRealTimers()
  })
  const advance = (ms: number) => act(() => vi.advanceTimersByTimeAsync(ms))

  it('stays silent for fast requests and appears only after 200 ms', async () => {
    const view = render(LoadingStatus, {
      pending: true,
      label: 'Loading record…',
    })
    await advance(199)
    expect(screen.queryByRole('status')).toBeNull()
    await view.rerender({ pending: false })
    await advance(1)
    expect(screen.queryByRole('status')).toBeNull()
    await view.rerender({ pending: true })
    await advance(199)
    expect(screen.queryByRole('status')).toBeNull()
    await advance(1)
    expect(screen.getByRole('status')).toHaveTextContent('Loading record…')
    await view.rerender({ pending: false })
    expect(screen.queryByRole('status')).toBeNull()
  })

  it('restarts the grace period for replacement and repeated identities', async () => {
    const view = render(LoadingStatus, { pending: true, requestKey: 'A' })
    await advance(200)
    expect(screen.getByRole('status')).toBeVisible()
    await view.rerender({ requestKey: 'B' })
    expect(screen.queryByRole('status')).toBeNull()
    await advance(150)
    await view.rerender({ requestKey: 'A' })
    await advance(199)
    expect(screen.queryByRole('status')).toBeNull()
    await advance(1)
    expect(screen.getByRole('status')).toBeVisible()
  })

  it('cancels its notice on unmount', async () => {
    const view = render(LoadingStatus, { pending: true })
    await advance(100)
    await view.unmount()
    await advance(200)
    expect(screen.queryByRole('status')).toBeNull()
    expect(vi.getTimerCount()).toBe(0)
  })
})
