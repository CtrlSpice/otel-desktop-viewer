import { onDestroy } from 'svelte'
import { createReadGroup } from '@/services/reads'
import { getDataChanges } from './data-changes.svelte'
import { getNavigationCounts } from './navigation-counts.svelte'

/** The page owns refresh; the persistent shell only exposes its action. */
export function createPageReadGroup(refresh: () => void | Promise<void>) {
  const totals = getNavigationCounts()
  const group = createReadGroup(() => {
    const completion = refresh()
    void totals?.refresh()
    return completion
  })
  const changes = getDataChanges()
  if (changes) onDestroy(changes.subscribeLive(() => void group.refresh()))
  return group
}
