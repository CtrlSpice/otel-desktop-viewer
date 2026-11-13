/** Run one refresh at a time, retaining one trailing refresh for newer writes. */
export function createRefreshCoordinator(refresh: () => Promise<void>) {
  let running: Promise<void> | undefined
  let dirty = false
  let disposed = false

  return {
    request(): Promise<void> {
      if (disposed) return Promise.resolve()
      dirty = true
      running ??= Promise.resolve().then(async () => {
        try {
          while (dirty && !disposed) {
            dirty = false
            await refresh()
          }
        } finally {
          running = undefined
        }
      })
      return running
    },
    dispose() {
      disposed = true
      dirty = false
    },
  }
}
