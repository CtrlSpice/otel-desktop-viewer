import { readable } from 'svelte/store'
import { Resource } from '@/services/resources/resource.svelte'

// The server is the authority; failures and missing flags close the feature.
export const evalsEnabled = readable(false, set => {
  if (!__EVALS_BUILT__) return
  const read = new Resource<boolean>()
  async function refresh() {
    await read.load(
      async signal => {
        const response = await fetch('/api/features', { signal })
        const value = response.ok ? await response.json() : null
        return value?.evals?.enabled === true
      },
      set,
      { onError: () => set(false) }
    )
  }
  void refresh()
  const timer = setInterval(() => void refresh(), 5000)
  return () => {
    clearInterval(timer)
    read.dispose()
  }
})
