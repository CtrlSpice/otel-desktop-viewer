import type { EventData, LogData, WaterfallEventData } from '@/types/api-types'

type EventAttributes =
  Pick<EventData, 'attributes'> | Pick<WaterfallEventData, 'summaryAttributes'>
type NamedEvent = EventAttributes & { name: string }

function attribute(event: EventAttributes, key: string) {
  return (
    'attributes' in event ? event.attributes : event.summaryAttributes
  ).find(item => item.key === key)?.value
}

export function toolEventError(event: EventAttributes) {
  const error = attribute(event, 'tool.is_error')
  return error?.kind === 'bool' && error.value
}

export function toolEventName(event: NamedEvent) {
  if (!/^(tool\.|gen_ai\.tool\.)/.test(event.name)) return event.name
  const name = attribute(event, 'gen_ai.tool.name')
  return name?.kind === 'string' && name.value ? name.value : event.name
}

/** Collector-created tool events carry their own explicit timestamp provenance. */
export function toolEventProvenance(event: NamedEvent) {
  if (!['tool.execution', 'tool.execution.start'].includes(event.name))
    return undefined
  for (const [key, runtime] of [
    ['pi.event.timestamp.source', 'Pi'],
    ['opencode.event.timestamp.source', 'OpenCode'],
  ]) {
    const source = attribute(event, key)
    if (source?.kind === 'string' && source.value)
      return {
        source: runtime,
        description: `Reconstructed from recorded ${runtime} tool evidence and stored as a parent-span event · timestamp: ${source.value}`,
      }
  }
  return undefined
}

/** Provenance for adapter-emitted log events; trace/span IDs remain on the log. */
export function toolLogProvenance(log: LogData) {
  if (!['tool.execution', 'tool.execution.start'].includes(log.eventName))
    return undefined
  const event = { name: log.eventName, attributes: log.attributes }
  const source = attribute(event, 'eval.evidence.source')
  const origin = attribute(event, 'eval.evidence.origin')
  if (
    source?.kind !== 'string' ||
    !['Pi', 'OpenCode'].includes(source.value) ||
    origin?.kind !== 'string' ||
    origin.value !== 'runtime-records'
  )
    return undefined
  const timestamp = attribute(
    event,
    source.value === 'Pi'
      ? 'pi.event.timestamp.source'
      : 'opencode.event.timestamp.source'
  )
  return {
    name: toolEventName(event),
    source: source.value,
    toolError: toolEventError(event),
    description: `Reconstructed from recorded ${source.value} tool evidence and stored as a log correlated by trace/span ID${timestamp?.kind === 'string' ? ` · timestamp: ${timestamp.value}` : ''}`,
  }
}
