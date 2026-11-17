import { untrack } from 'svelte'
import { SvelteMap } from 'svelte/reactivity'
import { Read, type ReadOptions } from './read.svelte'

/** Locally owned keyed reads; removing an entry ends its lifetime. */
export class ReadMap<K, T> extends SvelteMap<K, Read<T>> {
  #disposed = false
  constructor(private readonly options: ReadOptions = {}) {
    super()
  }

  for(key: K) {
    return untrack(() => {
      let read = this.get(key)
      if (!read) {
        read = new Read<T>(this.options)
        if (this.#disposed) read.dispose()
        else this.set(key, read)
      }
      return read
    })
  }

  get pending() {
    return [...this.values()].some(read => read.pending)
  }

  get requestKey() {
    return [...this.values()]
      .flatMap(read => (read.requestKey ? [read.requestKey] : []))
      .join(':')
  }

  override delete(key: K) {
    return untrack(() => {
      this.get(key)?.dispose()
      return super.delete(key)
    })
  }

  override clear() {
    untrack(() => {
      for (const read of this.values()) read.dispose()
      super.clear()
    })
  }

  dispose() {
    this.#disposed = true
    this.clear()
  }
}
