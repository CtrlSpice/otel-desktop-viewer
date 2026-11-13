import type { FirstOTLPRecord } from './otlp-file'
import { createRefreshCoordinator } from './refresh-coordinator'

export type ImportDestination = { id: string; spanID?: string }
class InvalidImportTarget extends Error {}

export async function lookupImportTarget(
  record: FirstOTLPRecord,
  signal: AbortSignal
): Promise<ImportDestination | null> {
  const response = await fetch(`/api/import-target/${record.signal}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: record.body,
    signal,
  })
  if (response.status >= 400 && response.status < 500)
    throw new InvalidImportTarget('The first record cannot be opened.')
  if (!response.ok) throw new Error('Could not look up the first record.')
  return response.json()
}

/** One navigation intent per selection; upload continues independently. */
export function createImportNavigation(options: {
  subscribe: (listener: () => void) => () => void
  lookup: typeof lookupImportTarget
  navigate: (record: FirstOTLPRecord, destination: ImportDestination) => void
  status: (message: string, pending: boolean) => void
}) {
  let cancel = () => {}
  return {
    select(fileName: string) {
      cancel()
      const controller = new AbortController()
      let record: FirstOTLPRecord | undefined
      let timeout: ReturnType<typeof setTimeout> | undefined
      const refresh = createRefreshCoordinator(async () => {
        if (!record || controller.signal.aborted) return
        const target = record
        try {
          const destination = await options.lookup(target, controller.signal)
          if (controller.signal.aborted || !destination) return
          stop()
          options.navigate(target, destination)
        } catch (error) {
          if (controller.signal.aborted) return
          if (error instanceof InvalidImportTarget) {
            stop()
            options.status(error.message, false)
          }
          // Reconnects and later commits retry transient query failures.
        }
      })
      const unsubscribe = options.subscribe(() => void refresh.request())
      function stop() {
        controller.abort()
        record = undefined
        clearTimeout(timeout)
        refresh.dispose()
        unsubscribe()
        options.status('', false)
      }
      cancel = stop
      options.status(`Opening the first record in ${fileName}…`, true)
      return {
        record(first: FirstOTLPRecord | undefined) {
          if (controller.signal.aborted) return
          if (!first) {
            stop()
            return
          }
          record = first
          void refresh.request()
        },
        submitted() {
          if (controller.signal.aborted) return
          if (!record) {
            stop()
            return
          }
          void refresh.request()
          // Acceptance does not prove storage. Bound the navigation wait only;
          // accepted data may still arrive after this intent has expired.
          timeout = setTimeout(() => {
            stop()
            options.status('The first record is not available yet.', false)
          }, 60_000)
        },
      }
    },
    cancel() {
      cancel()
    },
  }
}
