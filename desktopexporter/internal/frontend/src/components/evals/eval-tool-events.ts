import type { Attributes, SpanNode } from '@/types/api-types'
import type { EvalEventTool } from './model'

function value(attributes: Attributes, key: string) {
  const matches = attributes.filter(attribute => attribute.key === key)
  return matches.length === 1 && !matches[0]!.hasConflict
    ? matches[0]!.value
    : undefined
}
function text(attributes: Attributes, key: string) {
  const item = value(attributes, key)
  return item?.kind === 'string' ? item.value : undefined
}

/** Count recorded calls without projecting event evidence into span/tree rows.
 * IDs must be unique within the trace. A native tool span takes precedence over
 * an event with the same call ID, so mixed instrumentation cannot count twice. */
export function evalEventTools(nodes: SpanNode[]): EvalEventTool[] {
  const candidates = new Map<string, EvalEventTool[]>()
  const native = new Set<string>()
  for (const { spanData: span } of nodes) {
    const key = (id: string) => `${span.traceID.toLowerCase()}/${id}`
    const nativeID = text(span.attributes, 'gen_ai.tool.call.id')
    if (
      nativeID &&
      text(span.attributes, 'gen_ai.operation.name') === 'execute_tool'
    )
      native.add(key(nativeID))
    span.events.forEach((event, eventIndex) => {
      if (!['tool.execution', 'tool.execution.start'].includes(event.name))
        return
      const callID = text(event.attributes, 'gen_ai.tool.call.id')
      if (!callID) return
      const error = value(event.attributes, 'tool.is_error')
      const tools = candidates.get(key(callID)) ?? []
      tools.push({
        callID,
        eventIndex,
        parent: { traceID: span.traceID, spanID: span.spanID },
        failed: error?.kind === 'bool' ? error.value : undefined,
      })
      candidates.set(key(callID), tools)
    })
  }
  return [...candidates].flatMap(([key, tools]) =>
    tools.length === 1 && !native.has(key) ? tools : []
  )
}
