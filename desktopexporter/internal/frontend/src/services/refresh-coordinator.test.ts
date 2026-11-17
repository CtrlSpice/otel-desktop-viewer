import { expect, it, vi } from 'vitest'
import { createRefreshCoordinator } from './refresh-coordinator'

it('lets a slow refresh finish and combines newer requests into one trailing refresh', async () => {
  let finish!: () => void
  const refresh = vi
    .fn()
    .mockImplementationOnce(
      () => new Promise<void>(resolve => (finish = resolve))
    )
    .mockResolvedValue(undefined)
  const coordinator = createRefreshCoordinator(refresh)
  const first = coordinator.request()
  await Promise.resolve()
  const second = coordinator.request()
  for (let i = 0; i < 20; i++) void coordinator.request()
  expect(refresh).toHaveBeenCalledOnce()
  expect(second).toBe(first)
  finish()
  await first
  expect(refresh).toHaveBeenCalledTimes(2)
})

it('drops queued work when its page is disposed', async () => {
  let finish!: () => void
  const refresh = vi.fn(() => new Promise<void>(resolve => (finish = resolve)))
  const coordinator = createRefreshCoordinator(refresh)
  const pending = coordinator.request()
  await Promise.resolve()
  void coordinator.request()
  coordinator.dispose()
  finish()
  await pending
  await coordinator.request()
  expect(refresh).toHaveBeenCalledOnce()
})

it.each([false, true])(
  'combines requests while an existing read finishes (dispose: %s)',
  async dispose => {
    let finish!: () => void
    const pendingRead = new Promise<void>(resolve => (finish = resolve))
    const refresh = vi.fn().mockResolvedValue(undefined)
    const coordinator = createRefreshCoordinator(refresh, () => pendingRead)
    const pending = coordinator.request()
    await Promise.resolve()
    for (let i = 0; i < 20; i++) void coordinator.request()
    expect(refresh).not.toHaveBeenCalled()
    if (dispose) coordinator.dispose()
    finish()
    await pending
    expect(refresh).toHaveBeenCalledTimes(dispose ? 0 : 1)
  }
)
