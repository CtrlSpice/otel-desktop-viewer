import { tick, untrack } from 'svelte'
import { SvelteSet } from 'svelte/reactivity'
import { createRefreshCoordinator } from '../refresh-coordinator'
import type { ResourceMember } from './resource.svelte'

/** A refresh waits for actual reads and the dependent work their publication starts. */
export class ReadScope {
  #members = new SvelteSet<ResourceMember>()
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
    return [...this.#members].some(resource => resource.pending)
  }

  register(resource: ResourceMember) {
    return untrack(() => {
      if (this.#disposed) {
        resource.dispose()
        return () => {}
      }
      this.#members.add(resource)
      return () => {
        this.#members.delete(resource)
      }
    })
  }

  async settled() {
    await tick()
    while (!this.#disposed && this.pending) {
      await Promise.all([...this.#members].map(resource => resource.completion))
      await tick()
    }
  }

  refresh = () => this.#refresh.request()

  dispose() {
    this.#disposed = true
    this.#refresh.dispose()
    for (const resource of this.#members) resource.dispose()
  }
}
