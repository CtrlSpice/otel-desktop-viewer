import { tick, untrack } from 'svelte'
import { SvelteSet } from 'svelte/reactivity'
import { createRefreshCoordinator } from '../refresh-coordinator'
import type { ReadMember } from './read.svelte'

/** A refresh waits for actual reads and the dependent work their publication starts. */
export class ReadGroup {
  #members = new SvelteSet<ReadMember>()
  #disposed = false
  #refresh

  constructor(refresh: () => void | Promise<void> = () => {}) {
    this.#refresh = createRefreshCoordinator(
      async () => {
        await refresh()
        await this.settled()
      },
      () => this.settled()
    )
  }

  get pending() {
    return [...this.#members].some(read => read.pending)
  }

  register(read: ReadMember) {
    return untrack(() => {
      if (this.#disposed) {
        read.dispose()
        return () => {}
      }
      this.#members.add(read)
      return () => {
        this.#members.delete(read)
      }
    })
  }

  async settled() {
    await tick()
    while (!this.#disposed && this.pending) {
      await Promise.all([...this.#members].map(read => read.completion))
      await tick()
    }
  }

  refresh = () => this.#refresh.request()

  dispose() {
    this.#disposed = true
    this.#refresh.dispose()
    for (const read of this.#members) read.dispose()
  }
}
