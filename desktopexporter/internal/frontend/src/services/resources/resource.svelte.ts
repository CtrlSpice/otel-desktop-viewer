import { untrack } from 'svelte'

let nextRequestKey = 0

export type ResourceScope = {
  register(resource: ResourceMember): () => void
}

export type ResourceMember = {
  readonly pending: boolean
  readonly completion: Promise<void>
  dispose(): void
}

export type ResourceOptions = {
  scope?: ResourceScope | null
  errorMessage?: string
  formatError?: (error: Error) => string
  /** Keep the last failure visible until a replacement succeeds or fails. */
  retainError?: boolean
}

type Attempt = {
  controller: AbortController
  finish: () => void
  timer?: ReturnType<typeof setTimeout>
}

/** One replaceable read, retaining the last accepted value on loading or failure. */
export class Resource<T> {
  #data = $state.raw<T>()
  #error = $state<string | null>(null)
  #active = $state.raw<Attempt>()
  #owned: Attempt | undefined
  #requestKey = $state(0)
  #disposed = false
  #completion = Promise.resolve()
  #release: (() => void) | undefined

  constructor(private readonly options: ResourceOptions = {}) {
    this.#release = options.scope?.register(this)
  }

  get data() {
    return this.#data
  }
  get error() {
    return this.#error
  }
  get pending() {
    return this.#active !== undefined
  }
  get requestKey() {
    return this.#requestKey
  }
  get completion() {
    return this.#completion
  }

  /** Starting another read supersedes this owner's work, including delayed starts. */
  load(
    read: (
      signal: AbortSignal,
      publish: (update: () => void) => void
    ) => Promise<T>,
    publish?: (value: T) => void,
    options: { delayMs?: number; onError?: (error: Error) => void } = {}
  ): Promise<void> {
    return untrack(() => {
      this.cancel()
      if (this.#disposed) return Promise.resolve()
      const controller = new AbortController()
      let resolve!: () => void
      const done = new Promise<void>(finish => (resolve = finish))
      this.#completion = done
      const attempt: Attempt = {
        controller,
        finish: () => {
          if (this.#active === attempt) this.#active = undefined
          resolve()
        },
      }
      this.#active = attempt
      this.#owned = attempt
      this.#requestKey = ++nextRequestKey
      if (!this.options.retainError) this.#error = null
      const current = () =>
        this.#active === attempt && !controller.signal.aborted
      const run = async () => {
        if (!current()) return
        try {
          const value = await read(controller.signal, update => {
            if (current()) update()
          })
          if (!current()) return
          this.#error = null
          publish?.(value)
          if (current()) this.#data = value
        } catch (error) {
          if (!current()) return
          const failure =
            error instanceof Error
              ? error
              : new Error(this.options.errorMessage ?? 'Could not load data.', {
                  cause: error,
                })
          this.#error = this.options.formatError
            ? this.options.formatError(failure)
            : failure.message
          options.onError?.(failure)
        } finally {
          attempt.finish()
        }
      }
      if (options.delayMs) attempt.timer = setTimeout(run, options.delayMs)
      else void run()
      return done
    })
  }

  cancel() {
    untrack(() => {
      const attempt = this.#owned
      if (!attempt) return
      this.#owned = undefined
      this.#active = undefined
      if (attempt.timer !== undefined) clearTimeout(attempt.timer)
      attempt.controller.abort()
      attempt.finish()
    })
  }

  clear() {
    this.cancel()
    this.#data = undefined
    this.#error = null
  }

  clearError() {
    this.#error = null
  }

  dispose() {
    this.#disposed = true
    this.cancel()
    this.#release?.()
  }
}
