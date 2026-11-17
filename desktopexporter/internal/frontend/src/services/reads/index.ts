import { getContext, onDestroy, setContext } from 'svelte'
import { Read, type ReadOptions } from './read.svelte'
import { ReadGroup } from './group.svelte'
import { ReadMap } from './map.svelte'

const groupKey = Symbol('read-group')

export { Read, ReadGroup, ReadMap }
export type { ReadOptions }

export function createReadGroup(refresh?: () => void | Promise<void>) {
  const group = new ReadGroup(refresh)
  setContext(groupKey, group)
  onDestroy(() => group.dispose())
  return group
}

export function createRead<T>(options: ReadOptions = {}) {
  const group = getContext<ReadGroup | undefined>(groupKey)
  const read = new Read<T>({ group, ...options })
  onDestroy(() => read.dispose())
  return read
}

export function createReadMap<K, T>(options: ReadOptions = {}) {
  const group = getContext<ReadGroup | undefined>(groupKey)
  const reads = new ReadMap<K, T>({ group, ...options })
  onDestroy(() => reads.dispose())
  return reads
}
