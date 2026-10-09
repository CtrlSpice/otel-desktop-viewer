import { batchOTLPFile, scanOTLPFile, type ImportSignal } from './otlp-file'
import { telemetryAPI } from './telemetry-service'
import type { ImportFailure } from '@/types/import-types'

const signals: readonly ImportSignal[] = ['traces', 'logs', 'metrics']

function isReply(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
}

async function responseIssue(response: Response): Promise<string | undefined> {
  const text = await response.text()
  if (!response.ok) {
    try {
      const reply: unknown = JSON.parse(text)
      if (isReply(reply) && typeof reply.message === 'string')
        return reply.message
    } catch {
      /* Non-JSON proxy errors are still useful to the user. */
    }
    return text.trim() || `HTTP ${response.status}`
  }
  const reply: unknown = JSON.parse(text)
  if (!isReply(reply)) throw new Error('Invalid OTLP response')
  const partial = reply.partialSuccess
  if (partial === undefined) return
  if (!isReply(partial))
    throw new Error('Invalid OTLP partial-success response')
  const count =
    partial.rejectedSpans ??
    partial.rejectedLogRecords ??
    partial.rejectedDataPoints ??
    '0'
  if (!(
    (typeof count === 'string' && /^\d+$/.test(count)) ||
    (typeof count === 'number' && Number.isSafeInteger(count) && count >= 0)
  )) {
    throw new Error('Invalid OTLP rejected-record count')
  }
  if (
    partial.errorMessage !== undefined &&
    typeof partial.errorMessage !== 'string'
  ) {
    throw new Error('Invalid OTLP error message')
  }
  const rejected = BigInt(count)
  if (rejected > 0n || partial.errorMessage) {
    return partial.errorMessage || `The receiver rejected ${rejected} records.`
  }
}

/** All parsing finishes before the first request. Only the current batch is materialised. */
export async function importOTLPFile(
  file: File,
  pageURL: string,
  signal: AbortSignal
): Promise<string[]> {
  const plan = await scanOTLPFile(file, signal)
  const issues = [...plan.profiles, ...plan.invalid].map(
    issue => `${issue.reason} (byte ${issue.offset + 1})`
  )
  if (!signals.some(kind => plan[kind].length)) return issues

  let config: { otlpHttpPort: number }
  try {
    config = await telemetryAPI.getImportConfig(signal)
  } catch (error) {
    signal.throwIfAborted()
    issues.push(
      error instanceof Error ? error.message : 'Unable to locate OTLP receiver'
    )
    return issues
  }
  const endpoint = new URL(pageURL)
  // The CLI serves the viewer and OTLP on the same host; --http selects the port.
  endpoint.protocol = 'http:'
  endpoint.port = String(config.otlpHttpPort)
  endpoint.username = ''
  endpoint.password = ''
  endpoint.search = ''
  endpoint.hash = ''

  for (const kind of signals) {
    endpoint.pathname = `/v1/${kind}`
    for (const batch of batchOTLPFile(file, kind, plan[kind])) {
      signal.throwIfAborted()
      if ('issue' in batch) {
        issues.push(
          `${kind}: ${batch.issue.reason} (byte ${batch.issue.offset + 1})`
        )
        continue
      }
      try {
        const response = await fetch(endpoint.href, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: batch.body,
          signal,
        })
        const issue = await responseIssue(response)
        if (issue) issues.push(`${kind}: ${issue}`)
      } catch (error) {
        signal.throwIfAborted()
        issues.push(
          `${kind}: ${error instanceof Error ? error.message : 'Request failed'}`
        )
      }
    }
  }
  return issues
}

/** One serial queue also covers files selected while an earlier selection is running. */
export function createFileImporter(
  pageURL: string,
  onFailure: (failure: ImportFailure) => void
) {
  const controller = new AbortController()
  let pending = Promise.resolve()
  return {
    enqueue(files: readonly File[]): Promise<void> {
      for (const file of files) {
        pending = pending.then(async () => {
          if (controller.signal.aborted) return
          let issues: string[]
          try {
            issues = await importOTLPFile(file, pageURL, controller.signal)
          } catch (error) {
            if (controller.signal.aborted) return
            issues = [
              error instanceof Error ? error.message : 'Unable to read file',
            ]
          }
          if (!controller.signal.aborted && issues.length) {
            onFailure({
              fileName: file.name,
              reason: issues.join('\n'),
              occurredAt: BigInt(Date.now()) * 1_000_000n,
            })
          }
        })
      }
      return pending
    },
    cancel() {
      controller.abort()
    },
  }
}
