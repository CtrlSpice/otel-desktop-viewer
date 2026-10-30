import { expect, it, vi } from 'vitest'
import { evalRequestQueue } from './eval-request-queue'

it('bounds concurrency and starts the next request on any completion rather than a whole batch', async () => {
  const controller = new AbortController()
  const queue = evalRequestQueue(controller.signal)
  const finish: Array<() => void> = []
  const work = vi.fn(() => new Promise<void>(resolve => finish.push(resolve)))
  const tasks = Array.from({ length: 7 }, () => queue.run(work))
  await vi.waitFor(() => expect(work).toHaveBeenCalledTimes(4))
  finish[1]()
  await tasks[1]
  await vi.waitFor(() => expect(work).toHaveBeenCalledTimes(5))
  finish[4]()
  await vi.waitFor(() => expect(work).toHaveBeenCalledTimes(6))
  finish[5]()
  await vi.waitFor(() => expect(work).toHaveBeenCalledTimes(7))
  for (const resolve of finish) resolve()
  await Promise.all(tasks)
  controller.abort()
})

it('cancels queued work without starting it and rejects new work after abort', async () => {
  const controller = new AbortController()
  const queue = evalRequestQueue(controller.signal, 1)
  let finish!: () => void
  const first = queue.run(
    () =>
      new Promise<void>(resolve => {
        finish = resolve
      })
  )
  const work = vi.fn(async () => {})
  const queued = queue.run(work)
  const rejected = expect(queued).rejects.toMatchObject({ name: 'AbortError' })
  await vi.waitFor(() => expect(finish).toBeDefined())
  controller.abort()
  await rejected
  expect(work).not.toHaveBeenCalled()
  await expect(queue.run(work)).rejects.toMatchObject({ name: 'AbortError' })
  finish()
  await first
})
