// @vitest-environment jsdom
import { expect, it, vi } from 'vitest'
import { screen } from '@testing-library/svelte'
import { tick } from 'svelte'
import { renderWithContexts as render } from '@/test/render-helpers'
import EvalsPage from './EvalsPage.svelte'

const lazy = vi.hoisted(() => {
  let release!: () => void
  const pending = new Promise<void>(resolve => {
    release = resolve
  })
  return { release, pending, started: vi.fn() }
})
vi.mock('@/state/features', async () => {
  const { readable } = await import('svelte/store')
  return { evalsEnabled: readable(true) }
})
vi.mock('./EvalsWorkspace.svelte', async () => {
  lazy.started()
  await lazy.pending
  const placeholder =
    await import('@/components/evals/EvalDetailPlaceholder.svelte')
  return { default: placeholder.default }
})

it('starts the lazy workspace import immediately but delays Opening Evals feedback by 200ms', async () => {
  vi.useFakeTimers()
  try {
    const view = render(EvalsPage)
    await vi.advanceTimersByTimeAsync(0)
    expect(lazy.started).toHaveBeenCalledTimes(1)
    expect(screen.queryByText('Opening Evals…')).toBeNull()
    await vi.advanceTimersByTimeAsync(199)
    expect(screen.queryByText('Opening Evals…')).toBeNull()
    await vi.advanceTimersByTimeAsync(1)
    expect(screen.getByText('Opening Evals…')).toBeVisible()
    lazy.release()
    await vi.advanceTimersByTimeAsync(0)
    await tick()
    expect(screen.queryByText('Opening Evals…')).toBeNull()
    view.unmount()
  } finally {
    lazy.release()
    vi.useRealTimers()
  }
})
