import { ROOT_CONTEXT, trace, isSpanContextValid, type Context, type Attributes } from '@opentelemetry/api'
import { resourceFromAttributes } from '@opentelemetry/resources'
import { LoggerProvider, BatchLogRecordProcessor, type LogRecordExporter } from '@opentelemetry/sdk-logs'
import { OTLPLogExporter } from '@opentelemetry/exporter-logs-otlp-http'

export type ToolLog = {
  eventName: 'tool.execution' | 'tool.execution.start'
  attributes: Attributes
  timestamp: number
  observedTimestamp: number
}

type NativeAttribute = { key: string; value: { stringValue?: string; boolValue?: boolean; intValue?: string | number; doubleValue?: number } }
export type NativeToolSpan = { traceId: string; spanId: string; events?: { name: string; attributes?: NativeAttribute[] }[] }

/** Reuse complete received span events in the invocation trace. Matching names
 * or times do not establish identity; a unique call ID and every normalized tool
 * field must agree. Partial evidence still needs its enrichment log. */
export function missingToolLogs(records: ToolLog[], traceId: string, spans: NativeToolSpan[]): ToolLog[] {
  const events = spans.filter(span => span.traceId.toLowerCase() === traceId.toLowerCase()).flatMap(span => span.events ?? [])
  const matches = (attributes: NativeAttribute[], key: string, value: unknown) => {
    const found = attributes.filter(attribute => attribute.key === key)
    if (found.length !== 1) return false
    const received = found[0].value
    if (typeof value === 'string') return received.stringValue === value
    if (typeof value === 'boolean') return received.boolValue === value
    if (typeof value === 'number') return received.doubleValue === value || String(received.intValue) === String(value)
    return false
  }
  return records.filter(record => {
    const callID = record.attributes['gen_ai.tool.call.id']
    if (typeof callID !== 'string' || !callID) return true
    const candidates = events.filter(event => event.name === record.eventName && matches(event.attributes ?? [], 'gen_ai.tool.call.id', callID))
    if (candidates.length !== 1) return true
    return !Object.entries(record.attributes).filter(([key]) => /^(tool\.|gen_ai\.tool\.)/.test(key))
      .every(([key, value]) => matches(candidates[0].attributes ?? [], key, value))
  })
}

/** Adapter-owned logs use the captured invocation context, not whichever span
 * happens to be active when the runtime finishes. They never create tool spans. */
export async function exportToolLogs(records: ToolLog[], invocation: Context, source: 'Pi' | 'OpenCode', evidenceID: string,
  endpoint = process.env.OTEL_EVAL_OBSERVER_HTTP) {
  if (!endpoint || !records.length) return
  const span = trace.getSpanContext(invocation)
  if (!span || !isSpanContextValid(span)) throw new Error('Tool logs require the recorded invocation span context')
  const url = new URL(endpoint)
  if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password || url.search || url.hash)
    throw new Error('Tool log observer must be an HTTP(S) URL without credentials, query or fragment')
  const exporter = new OTLPLogExporter({ url: url.href.replace(/\/$/, '') + '/v1/logs', timeoutMillis: 30000 })
  const failures: Error[] = []
  const checkedExporter: LogRecordExporter = {
    export(logs, done) {
      exporter.export(logs, result => {
        if (result.code !== 0) failures.push(result.error ?? new Error('Tool log export failed'))
        done(result)
      })
    },
    shutdown: () => exporter.shutdown(),
    forceFlush: () => exporter.forceFlush(),
  }
  const provider = new LoggerProvider({
    resource: resourceFromAttributes({ 'service.name': 'viewer-eval', 'eval.evidence.id': evidenceID }),
    logRecordLimits: { attributeCountLimit: 128, attributeValueLengthLimit: Infinity },
    processors: [new BatchLogRecordProcessor({ exporter: checkedExporter, maxQueueSize: Math.max(2048, records.length), maxExportBatchSize: 512 })],
  })
  const logger = provider.getLogger('viewer.eval.tool-evidence', '1')
  const context = trace.setSpanContext(ROOT_CONTEXT, span)
  try {
    for (const record of records) logger.emit({
      ...record, context,
      body: record.attributes['gen_ai.tool.name'],
      attributes: { ...record.attributes, 'eval.evidence.source': source, 'eval.evidence.kind': 'tool',
        'eval.evidence.origin': 'runtime-records' },
    })
    await provider.forceFlush()
    if (failures.length) throw new AggregateError(failures, 'Tool evidence log export failed')
  } finally { await provider.shutdown() }
}
