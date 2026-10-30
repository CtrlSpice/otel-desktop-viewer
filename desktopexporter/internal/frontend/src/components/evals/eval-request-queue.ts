/** Bound independent requests without making their publication wait for a batch. */
export function evalRequestQueue(signal: AbortSignal, concurrency = 4) {
  let active = 0
  const waiting: Array<() => void> = []
  function advance() {
    while (active < concurrency && waiting.length) waiting.shift()!()
  }
  signal.addEventListener(
    'abort',
    () => {
      for (const cancel of waiting.splice(0)) cancel()
    },
    { once: true }
  )
  return {
    run<T>(request: () => Promise<T>): Promise<T> {
      if (signal.aborted) return Promise.reject(signal.reason)
      return new Promise((resolve, reject) => {
        waiting.push(() => {
          if (signal.aborted) {
            reject(signal.reason)
            return
          }
          active++
          void Promise.resolve()
            .then(() => {
              signal.throwIfAborted()
              return request()
            })
            .then(resolve, reject)
            .finally(() => {
              active--
              advance()
            })
        })
        advance()
      })
    },
  }
}

export type EvalRequestPart<T> =
  { status: 'pending' } | { status: 'ready'; value: T } | { status: 'failed' }
