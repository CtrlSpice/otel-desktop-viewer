import { getContext, onDestroy, setContext } from 'svelte'
import { Resource, type ResourceOptions } from './resource.svelte'
import { ReadScope } from './scope.svelte'
import { ResourceMap } from './map.svelte'

const scopeKey = Symbol('read-scope')

export { Resource, ReadScope, ResourceMap }
export type { ResourceOptions }

export function createReadScope(refresh?: () => void | Promise<void>) {
  const scope = new ReadScope(refresh)
  setContext(scopeKey, scope)
  onDestroy(() => scope.dispose())
  return scope
}

export function createResource<T>(options: ResourceOptions = {}) {
  const scope = getContext<ReadScope | undefined>(scopeKey)
  const resource = new Resource<T>({ scope, ...options })
  onDestroy(() => resource.dispose())
  return resource
}

export function createResourceMap<K, T>(options: ResourceOptions = {}) {
  const scope = getContext<ReadScope | undefined>(scopeKey)
  const resources = new ResourceMap<K, T>({ scope, ...options })
  onDestroy(() => resources.dispose())
  return resources
}
