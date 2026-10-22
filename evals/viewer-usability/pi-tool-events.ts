import type { Attributes, Span, TimeInput } from '@opentelemetry/api'

type RecordValue = Record<string, unknown>
type Received = { value: RecordValue; observedAt: number }

const object = (value: unknown): RecordValue | undefined =>
  value !== null && typeof value === 'object' && !Array.isArray(value)
    ? value as RecordValue : undefined
const text = (value: unknown) => typeof value === 'string' && value ? value : undefined

/** Rich events on the actual provider span, never synthetic tool spans.
 * Pi 1.3.0 tool start/end records have no source timestamp. A unique toolResult
 * message records its creation time after execution; it is not an execution start.
 * Without that message, retain the collector's observed time and label it as such.
 * Original records remain in events.jsonl, including ambiguous and partial calls. */
export class PiToolEvents {
  private records: Received[] = []

  observe(value: RecordValue, observedAt = Date.now()) {
    if (value.type === 'tool_execution_start' || value.type === 'tool_execution_end' ||
      (value.type === 'message_end' && object(value.message)?.role === 'toolResult'))
      this.records.push({ value, observedAt })
  }

  flush(span: Pick<Span, 'addEvent'> | undefined) {
    if (!span) return
    const records = this.records
    this.records = []
    const starts = records.filter(record => record.value.type === 'tool_execution_start')
    const ends = records.filter(record => record.value.type === 'tool_execution_end')
    const messages = records.filter(record => record.value.type === 'message_end')
    const joined = new Set<Received>()
    const same = (a: RecordValue, b: RecordValue) =>
      !!text(a.toolCallId) && a.toolCallId === b.toolCallId &&
      !!text(a.toolName) && a.toolName === b.toolName &&
      a.parentToolCallId === b.parentToolCallId
    for (const end of ends) {
      const value = end.value
      const id = text(value.toolCallId)
      const unambiguousEnd = !!id && ends.filter(item => item.value.toolCallId === id).length === 1
      const candidates = starts.filter(item => item.value.toolCallId === id)
      const start = unambiguousEnd && candidates.length === 1 && same(value, candidates[0]!.value)
        ? candidates[0] : undefined
      const results = messages.filter(item => object(item.value.message)?.toolCallId === id)
      const message = unambiguousEnd && results.length === 1 ? object(results[0]!.value.message) : undefined
      const timestamp = message && same(value, message) && message.isError === value.isError &&
        message.durationMs === value.durationMs && Number.isSafeInteger(message.timestamp) &&
        (message.timestamp as number) >= 0 ? message.timestamp as number : undefined
      const attributes = this.attributes(value)
      if (start) {
        joined.add(start)
        attributes['pi.tool.start.raw'] = JSON.stringify(start.value)
        if (start.value.args !== undefined) attributes['tool.input'] = JSON.stringify(start.value.args)
      }
      if (value.result !== undefined) attributes['tool.output'] = JSON.stringify(value.result)
      if (typeof value.isError === 'boolean') attributes['tool.is_error'] = value.isError
      if (typeof value.durationMs === 'number' && Number.isFinite(value.durationMs) && value.durationMs >= 0)
        attributes['tool.duration_ms'] = value.durationMs
      if (timestamp !== undefined) attributes['pi.tool.result.raw'] = JSON.stringify(results[0]!.value)
      attributes['pi.event.timestamp.source'] = timestamp === undefined ? 'collector.observed' : 'toolResult.timestamp'
      span.addEvent('tool.execution', attributes, (timestamp ?? end.observedAt) as TimeInput)
    }
    for (const start of starts) {
      if (joined.has(start)) continue
      const attributes = this.attributes(start.value)
      if (start.value.args !== undefined) attributes['tool.input'] = JSON.stringify(start.value.args)
      attributes['pi.event.timestamp.source'] = 'collector.observed'
      span.addEvent('tool.execution.start', attributes, start.observedAt)
    }
  }

  private attributes(value: RecordValue): Attributes {
    const attributes: Attributes = {
      'pi.event.type': value.type as string,
      'pi.event.raw': JSON.stringify(value),
      'pi.runtime.version': '1.3.0',
    }
    const id = text(value.toolCallId)
    const name = text(value.toolName)
    if (id) attributes['gen_ai.tool.call.id'] = id
    if (name) attributes['gen_ai.tool.name'] = name
    if (text(value.parentToolCallId)) attributes['pi.tool.parent_call_id'] = value.parentToolCallId as string
    return attributes
  }
}
