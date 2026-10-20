import type { SpanData } from '@/types/api-types'

type SpanIdentity = Pick<SpanData, 'traceID' | 'spanID'>

export function spanExportURL(span: SpanIdentity): string {
  return `/export/traces/${encodeURIComponent(span.traceID)}?spanId=${encodeURIComponent(span.spanID)}`
}

/** Read the same complete OTLP text as Download, without parsing numeric values
 * or reconstructing resource/scope envelopes from the reduced display model. */
export async function spanCopyJSON(
  span: SpanIdentity,
  signal: AbortSignal
): Promise<string> {
  const contentType = 'application/json'
  const response = await fetch(spanExportURL(span), {
    signal,
    headers: { Accept: contentType },
  })
  if (!response.ok) {
    throw new Error(
      (await response.text()).trim() || `Export failed (${response.status})`
    )
  }
  if (
    response.headers.get('Content-Type')?.split(';')[0]?.trim() !== contentType
  ) {
    throw new Error('The viewer returned an unexpected export format')
  }
  const text = await response.text()
  signal.throwIfAborted()
  return text
}
