import { expect, it, vi } from 'vitest'
import { Read } from './read.svelte'
import { ReadGroup } from './group.svelte'
import { ReadMap } from './map.svelte'

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (error: unknown) => void
  const promise = new Promise<T>((yes, no) => {
    resolve = yes
    reject = no
  })
  return { promise, resolve, reject }
}

it('retains the accepted reading through replacement failures and rejects late completions', async () => {
  const read = new Read<{ id: string }>()
  await read.load(async () => ({ id: 'A' }))
  const failed = deferred<{ id: string }>()
  const pending = read.load(() => failed.promise)
  expect(read.pending).toBe(true)
  expect(read.data).toEqual({ id: 'A' })
  failed.reject(new Error('B failed'))
  await pending
  expect(read.error).toBe('B failed')
  expect(read.data).toEqual({ id: 'A' })
  const late = deferred<{ id: string }>()
  let signal!: AbortSignal
  const publish = vi.fn()
  const obsolete = read.load(s => {
    signal = s
    return late.promise
  }, publish)
  await read.load(async () => ({ id: 'C' }))
  await obsolete
  expect(signal.aborted).toBe(true)
  late.resolve({ id: 'B' })
  await Promise.resolve()
  expect(publish).not.toHaveBeenCalled()
  expect(read.data).toEqual({ id: 'C' })
  expect(read.pending).toBe(false)
})

it('tracks dependent reads automatically and coalesces refresh requests', async () => {
  const initial = deferred<string>()
  const child = deferred<string>()
  let root!: Read<string>
  let nested!: Read<string>
  const refresh = vi.fn(() => {
    void root.load(async () => 'refreshed')
  })
  const group = new ReadGroup(refresh)
  root = new Read({ group })
  nested = new Read({ group })
  void root.load(
    () => initial.promise,
    () => {
      void nested.load(() => child.promise)
    }
  )
  const refreshing = group.refresh()
  for (let i = 0; i < 10; i++) void group.refresh()
  await Promise.resolve()
  expect(refresh).not.toHaveBeenCalled()
  initial.resolve('initial')
  await Promise.resolve()
  expect(group.pending).toBe(true)
  expect(refresh).not.toHaveBeenCalled()
  child.resolve('child')
  await refreshing
  expect(refresh).toHaveBeenCalledTimes(1)
  expect(group.pending).toBe(false)
  group.dispose()
})

it('releases group completion on teardown even when a transport ignores abort', async () => {
  const refresh = vi.fn()
  const group = new ReadGroup(refresh)
  const read = new Read<string>({ group })
  const work = deferred<string>()
  let signal!: AbortSignal
  void read.load(s => {
    signal = s
    return work.promise
  })
  const refreshing = group.refresh()
  group.dispose()
  await refreshing
  expect(signal.aborted).toBe(true)
  expect(read.pending).toBe(false)
  expect(refresh).not.toHaveBeenCalled()
})

it('cancels keyboard delay without starting the obsolete read', async () => {
  vi.useFakeTimers()
  try {
    const read = new Read<string>()
    const obsolete = vi.fn(async () => 'obsolete')
    const delayed = read.load(obsolete, undefined, { delayMs: 150 })
    expect(read.pending).toBe(true)
    await read.load(async () => 'current')
    await delayed
    await vi.advanceTimersByTimeAsync(150)
    expect(obsolete).not.toHaveBeenCalled()
    expect(read.data).toBe('current')
  } finally {
    vi.useRealTimers()
  }
})

it('gives different keyed owners distinct loading identities, including recreated entries', async () => {
  const reads = new ReadMap<string, string>()
  const a = reads.for('A')
  const b = reads.for('B')
  void a.load(() => new Promise(() => {}))
  void b.load(() => new Promise(() => {}))
  const first = a.requestKey
  expect(b.requestKey).not.toBe(first)
  reads.delete('A')
  const replacement = reads.for('A')
  void replacement.load(() => new Promise(() => {}))
  expect(replacement.requestKey).not.toBe(first)
  expect(replacement.requestKey).not.toBe(b.requestKey)
  reads.dispose()
  await Promise.all([a.completion, b.completion, replacement.completion])
})

it('ends completed and active ownership on group disposal and prevents later keyed reads', async () => {
  const group = new ReadGroup()
  const reads = new ReadMap<string, string>({ group })
  let completed!: AbortSignal
  await reads.for('done').load(async signal => {
    completed = signal
    return 'received'
  })
  const pending = deferred<string>()
  let active!: AbortSignal
  const waiting = reads.for('pending').load(signal => {
    active = signal
    return pending.promise
  })
  group.dispose()
  await waiting
  expect(completed.aborted).toBe(true)
  expect(active.aborted).toBe(true)
  const late = vi.fn(async () => 'late')
  await reads.for('late').load(late)
  expect(late).not.toHaveBeenCalled()
  reads.dispose()
  await reads.for('after-unmount').load(late)
  expect(late).not.toHaveBeenCalled()
  expect(reads.size).toBe(0)
})

it('rejects late progressive publication and keeps a replacement started during acceptance', async () => {
  const read = new Read<string>()
  const obsolete = deferred<string>()
  let progress!: (update: () => void) => void
  const first = read.load((_signal, publish) => {
    progress = publish
    return obsolete.promise
  })
  const next = deferred<string>()
  await read.load(
    async () => 'second',
    () => {
      void read.load(() => next.promise)
    }
  )
  await first
  const publish = vi.fn()
  progress(publish)
  obsolete.resolve('obsolete')
  await Promise.resolve()
  expect(publish).not.toHaveBeenCalled()
  expect(read.pending).toBe(true)
  expect(read.data).toBeUndefined()
  next.resolve('third')
  await read.completion
  expect(read.data).toBe('third')
})
