import { onDestroy } from 'svelte'
import { createReadScope } from '@/services/resources'
import { getDataChanges } from './data-changes.svelte'
import { getNavigationCounts } from './navigation-counts.svelte'

/** The page owns refresh; the persistent shell only exposes its action. */
export function createPageResources(refresh: () => void | Promise<void>) {
  const totals = getNavigationCounts()
  const scope = createReadScope(() => {
    const completion = refresh()
    void totals?.refresh()
    return completion
  })
  const changes = getDataChanges()
  if (changes) onDestroy(changes.subscribeLive(() => void scope.refresh()))
  return scope
}
