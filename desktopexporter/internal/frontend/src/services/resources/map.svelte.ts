import { untrack } from 'svelte'
import { SvelteMap } from 'svelte/reactivity'
import { Resource, type ResourceOptions } from './resource.svelte'

/** Locally owned keyed reads; removing an entry ends its lifetime. */
export class ResourceMap<K, T> extends SvelteMap<K, Resource<T>> {
  #disposed = false
  constructor(private readonly options: ResourceOptions = {}) {
    super()
  }

  for(key: K) {
    return untrack(() => {
      let resource = this.get(key)
      if (!resource) {
        resource = new Resource<T>(this.options)
        if (this.#disposed) resource.dispose()
        else this.set(key, resource)
      }
      return resource
    })
  }

  get pending() {
    return [...this.values()].some(resource => resource.pending)
  }

  get requestKey() {
    return [...this.values()]
      .flatMap(resource => (resource.requestKey ? [resource.requestKey] : []))
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
      for (const resource of this.values()) resource.dispose()
      super.clear()
    })
  }

  dispose() {
    this.#disposed = true
    this.clear()
  }
}
