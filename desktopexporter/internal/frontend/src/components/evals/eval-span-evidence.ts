import type {
  Attribute,
  Attributes,
  LogData,
  SpanData,
  SpanNode,
} from '@/types/api-types'
import { referenceKey } from './eval-traces'
import {
  toolEventProvenance,
  toolEventError,
  toolEventName,
  toolLogProvenance,
} from '@/utils/tool-event'

export type EvalSpanEvidence = {
  id: string
  source: string
  fields: Attribute[]
  toolError?: boolean
  sourceDescription?: string
}

const toolKey = (key: string) => /^(tool\.|gen_ai\.tool\.)/.test(key)
const graderKey = (key: string) =>
  /^(gen_ai\.evaluation\.|promptfoo\.grader\.)/.test(key)
const evidenceKey = (key: string) => toolKey(key) || graderKey(key)
const evidenceEvent = (name: string) =>
  /^(tool\.|gen_ai\.tool\.|gen_ai\.evaluation\.)/.test(name)

function attributeString(attributes: Attributes, key: string) {
  const values = attributes.filter(attribute => attribute.key === key)
  const attribute = values.length === 1 ? values[0] : undefined
  return attribute?.value.kind === 'string' && !attribute.hasConflict
    ? attribute.value.value
    : undefined
}

const stringAttribute = (span: SpanData, key: string) =>
  attributeString(span.attributes, key)
export function recordedToolCallID(attributes: Attributes) {
  for (const key of ['gen_ai.tool.call.id', 'tool_use_id']) {
    const matches = attributes.filter(attribute => attribute.key === key)
    if (
      matches.length > 1 ||
      matches.some(
        attribute => attribute.hasConflict || attribute.value.kind !== 'string'
      )
    )
      return undefined
  }
  const canonical = attributeString(attributes, 'gen_ai.tool.call.id')
  const alias = attributeString(attributes, 'tool_use_id')
  if (canonical && alias && canonical !== alias) return undefined
  return canonical || alias
}

export function uniqueToolCallID(span: SpanData, nodes: Map<string, SpanNode>) {
  const id = recordedToolCallID(span.attributes)
  if (!id || stringAttribute(span, 'gen_ai.operation.name') !== 'execute_tool')
    return undefined
  const matches = [...nodes.values()].filter(
    node =>
      node.spanData.traceID.toLowerCase() === span.traceID.toLowerCase() &&
      stringAttribute(node.spanData, 'gen_ai.operation.name') ===
        'execute_tool' &&
      recordedToolCallID(node.spanData.attributes) === id
  )
  return matches.length === 1 ? id : undefined
}

/** Ancestor logs require a unique recorded call ID, never time or command matching. */
export function toolLogOwners(
  spans: SpanData[],
  nodes: Map<string, SpanNode>,
  id?: string
) {
  const owners = [...spans]
  if (!id) return owners
  const seen = new Set(owners.map(referenceKey))
  let span: SpanData | undefined = spans[0]
  while (span?.parentSpanID) {
    const key = referenceKey({
      traceID: span.traceID,
      spanID: span.parentSpanID,
    })
    if (seen.has(key)) break
    seen.add(key)
    span = nodes.get(key)?.spanData
    if (span) owners.push(span)
  }
  return owners
}

/** Include only recorded descendants of this call, stopping at another tool call. */
export function toolEvidenceSpans(
  span: SpanData,
  nodes: Map<string, SpanNode>
): SpanData[] {
  if (stringAttribute(span, 'gen_ai.operation.name') !== 'execute_tool')
    return [span]
  const result = [span]
  const seen = new Set([referenceKey(span)])
  for (let index = 0; index < result.length; index++) {
    const parent = result[index]!
    for (const { spanData: child } of nodes.values()) {
      if (
        child.traceID.toLowerCase() !== span.traceID.toLowerCase() ||
        child.parentSpanID?.toLowerCase() !== parent.spanID.toLowerCase() ||
        seen.has(referenceKey(child)) ||
        stringAttribute(child, 'gen_ai.operation.name') === 'execute_tool'
      )
        continue
      seen.add(referenceKey(child))
      result.push(child)
    }
  }
  return result
}

/** Received values only. Names select evidence; composite span IDs establish ownership. */
export function evalSpanEvidence(
  spans: SpanData[],
  logs: LogData[] = [],
  toolCallID?: string
): EvalSpanEvidence[] {
  const records: EvalSpanEvidence[] = []
  const owners = new Set(spans.map(referenceKey))
  for (const span of spans) {
    const key = referenceKey(span)
    const claude = span.scope.name === 'com.anthropic.claude_code.tracing'
    const fields = span.attributes.filter(
      attribute =>
        evidenceKey(attribute.key) ||
        (claude && /^(tool_|full_command$)/.test(attribute.key))
    )
    if (fields.length)
      records.push({
        id: `${key}/attributes`,
        source: `${span.name} · Span`,
        fields,
      })
    span.events.forEach((event, index) => {
      const provenance = toolEventProvenance(event)
      const fields = evidenceEvent(event.name)
        ? event.attributes
        : event.attributes.filter(attribute => evidenceKey(attribute.key))
      if (fields.length)
        records.push({
          id: `${key}/event/${index}`,
          source: `${toolEventName(event)} · Span event ${index + 1}`,
          sourceDescription: provenance
            ? `${provenance.description} · Event name: ${event.name}`
            : undefined,
          toolError: toolEventError(event),
          fields,
        })
    })
  }
  for (const log of logs) {
    const provenance = toolLogProvenance(log)
    const exact =
      !!log.traceID &&
      !!log.spanID &&
      owners.has(referenceKey({ traceID: log.traceID, spanID: log.spanID }))
    const linkedCall =
      !!toolCallID &&
      log.traceID?.toLowerCase() === spans[0]?.traceID.toLowerCase() &&
      recordedToolCallID(log.attributes) === toolCallID
    if (!exact && !linkedCall) continue
    if (
      toolCallID &&
      recordedToolCallID(log.attributes) &&
      recordedToolCallID(log.attributes) !== toolCallID
    )
      continue
    const named = evidenceEvent(log.eventName)
    const claude =
      log.body.kind === 'string' && log.body.value === 'claude_code.tool_result'
    const fields = named
      ? [...log.attributes]
      : log.attributes.filter(
          attribute =>
            evidenceKey(attribute.key) ||
            (claude &&
              /^(tool_|success$|error$|duration_ms$)/.test(attribute.key))
        )
    const bodyFields =
      log.body.kind === 'map'
        ? log.body.value.filter(attribute => evidenceKey(attribute.key))
        : []
    if (named || fields.length || bodyFields.length) {
      // Keep the complete typed body, rather than parsing strings or extracting lossy snippets.
      if (log.body.kind !== 'empty')
        fields.unshift({ key: 'body', value: log.body })
      if (fields.length)
        records.push({
          id: `log/${log.logRef}`,
          source: provenance
            ? `${provenance.name} · Log event · ${provenance.source}`
            : `${log.eventName ? `${log.eventName} · ` : ''}Log`,
          sourceDescription: provenance?.description,
          toolError: provenance?.toolError,
          fields,
        })
    }
  }
  return records
}
