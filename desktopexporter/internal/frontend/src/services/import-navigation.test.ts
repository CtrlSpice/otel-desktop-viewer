import { afterEach, expect, it, vi } from 'vitest'
import {
  createImportNavigation,
  type ImportDestination,
} from './import-navigation'
import type { FirstOTLPRecord } from './otlp-file'

afterEach(() => vi.useRealTimers())

const record: FirstOTLPRecord = { signal: 'logs', body: new Blob(['{}']) }
function setup(
  lookup = vi
    .fn<() => Promise<ImportDestination | null>>()
    .mockResolvedValue(null)
) {
  const listeners = new Set<() => void>()
  const navigate = vi.fn()
  const status = vi.fn()
  const onFailure = vi.fn()
  const navigation = createImportNavigation({
    subscribe(listener) {
      listeners.add(listener)
      return () => {
        listeners.delete(listener)
      }
    },
    lookup,
    navigate,
    status,
    onFailure,
  })
  return {
    navigation,
    lookup,
    navigate,
    status,
    onFailure,
    commit: () => {
      for (const listener of listeners) listener()
    },
  }
}

it('retries missing targets on commits and navigates only once without Live', async () => {
  const test = setup()
  test.navigation.select('first.json').record(record)
  await vi.waitFor(() => expect(test.lookup).toHaveBeenCalledOnce())
  expect(test.navigate).not.toHaveBeenCalled()
  test.lookup.mockResolvedValue({ id: 'stored-log' })
  test.commit()
  await vi.waitFor(() =>
    expect(test.navigate).toHaveBeenCalledWith(record, { id: 'stored-log' })
  )
  test.commit()
  expect(test.lookup).toHaveBeenCalledTimes(2)
})

it('ignores an old lookup after a new file selection', async () => {
  let finish!: (destination: ImportDestination) => void
  const lookup = vi
    .fn<() => Promise<ImportDestination | null>>()
    .mockImplementationOnce(() => new Promise(resolve => (finish = resolve)))
    .mockResolvedValue({ id: 'new-log' })
  const test = setup(lookup)
  const first = test.navigation.select('first.json')
  first.record(record)
  await Promise.resolve()
  test.navigation.select('second.json').record(record)
  finish({ id: 'old-log' })
  first.submitted()
  await vi.waitFor(() => expect(test.navigate).toHaveBeenCalledOnce())
  expect(test.navigate).toHaveBeenCalledWith(record, { id: 'new-log' })
})

it('waits for a commit through queueing, upload and delayed storage', async () => {
  vi.useFakeTimers()
  const test = setup()
  const selection = test.navigation.select('first.json')
  await vi.advanceTimersByTimeAsync(120_000)
  selection.record(record)
  await vi.advanceTimersByTimeAsync(120_000)
  selection.submitted()
  await vi.advanceTimersByTimeAsync(120_000)
  expect(test.navigate).not.toHaveBeenCalled()
  expect(test.status).toHaveBeenLastCalledWith(
    'Opening the first record in first.json…'
  )
  test.lookup.mockResolvedValue({ id: 'stored-log' })
  test.commit()
  await vi.advanceTimersByTimeAsync(0)
  expect(test.navigate).toHaveBeenCalledExactlyOnceWith(record, {
    id: 'stored-log',
  })
  expect(test.status).toHaveBeenLastCalledWith('')
})

it('cancels a pending lookup and ignores later commits', async () => {
  let finish!: (destination: ImportDestination) => void
  const test = setup(
    vi.fn(() => new Promise<ImportDestination>(resolve => (finish = resolve)))
  )
  const selection = test.navigation.select('first.json')
  selection.record(record)
  await Promise.resolve()
  test.navigation.cancel()
  finish({ id: 'stored-log' })
  await Promise.resolve()
  selection.submitted()
  const calls = test.lookup.mock.calls.length
  test.commit()
  await Promise.resolve()
  expect(test.lookup).toHaveBeenCalledTimes(calls)
  expect(test.navigate).not.toHaveBeenCalled()
  expect(test.status).toHaveBeenLastCalledWith('')
})
