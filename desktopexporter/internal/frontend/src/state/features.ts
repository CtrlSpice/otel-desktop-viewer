import { readable } from 'svelte/store'

// The server is the authority; failures and missing flags close the feature.
export const evalsEnabled = readable(false, set => {
  if (!__EVALS_BUILT__) return
  let controller: AbortController | undefined
  async function refresh() {
    controller?.abort()
    const request = new AbortController()
    controller = request
    try {
      const response = await fetch('/api/features', { signal: request.signal })
      const value = response.ok ? await response.json() : null
      if (!request.signal.aborted) set(value?.evals?.enabled === true)
    } catch {
      if (!request.signal.aborted) set(false)
    }
  }
  void refresh()
  const timer = setInterval(() => void refresh(), 5000)
  return () => {
    clearInterval(timer)
    controller?.abort()
  }
})
