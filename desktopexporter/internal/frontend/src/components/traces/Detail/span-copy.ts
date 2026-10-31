import type { SpanData } from '@/types/api-types'
import { readOTLPURL } from '@/services/export-service'

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
  return readOTLPURL(spanExportURL(span), signal)
}
