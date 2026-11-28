import type { FirstOTLPRecord } from './otlp-file'
import { createRefreshCoordinator } from '../refresh-coordinator'
import { Read } from '../reads/read.svelte'
import type { ImportFailure } from '@/types/import-types'

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
  status: (message: string) => void
  onFailure: (failure: ImportFailure) => void
}) {
  let cancel = () => {}
  return {
    select(fileName: string) {
      cancel()
      const read = new Read<ImportDestination | null>()
      let active = true
      let record: FirstOTLPRecord | undefined
      const refresh = createRefreshCoordinator(async () => {
        if (!record || !active) return
        const target = record
        await read.load(
          signal => options.lookup(target, signal),
          destination => {
            if (!destination) return
            stop()
            options.navigate(target, destination)
          },
          {
            onError: error => {
              if (error instanceof InvalidImportTarget) {
                stop()
                options.onFailure({
                  fileName,
                  reason: error.message,
                  occurredAt: BigInt(Date.now()) * 1_000_000n,
                })
              }
              // Reconnects and later commits retry transient query failures.
            },
          }
        )
      })
      const unsubscribe = options.subscribe(() => void refresh.request())
      function stop() {
        active = false
        read.dispose()
        record = undefined
        refresh.dispose()
        unsubscribe()
        options.status('')
      }
      cancel = stop
      options.status(`Opening the first record in ${fileName}…`)
      return {
        record(first: FirstOTLPRecord | undefined) {
          if (!active) return
          if (!first) {
            stop()
            return
          }
          record = first
          void refresh.request()
        },
        submitted() {
          if (!active) return
          if (!record) {
            stop()
            return
          }
          void refresh.request()
        },
      }
    },
    cancel() {
      cancel()
    },
  }
}
