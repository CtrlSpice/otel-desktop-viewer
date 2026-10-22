import type { EventData } from '@/types/api-types'

function attribute(event: EventData, key: string) {
  const values = event.attributes.filter(item => item.key === key)
  return values.length === 1 && !values[0]!.hasConflict
    ? values[0]!.value
    : undefined
}

export function toolEventError(event: EventData) {
  const error = attribute(event, 'tool.is_error')
  return error?.kind === 'bool' && error.value
}

export function toolEventName(event: EventData) {
  if (!/^(tool\.|gen_ai\.tool\.)/.test(event.name)) return event.name
  const name = attribute(event, 'gen_ai.tool.name')
  return name?.kind === 'string' && name.value ? name.value : event.name
}

/** Collector-created tool events carry their own explicit timestamp provenance. */
export function toolEventAnnotation(event: EventData) {
  if (!['tool.execution', 'tool.execution.start'].includes(event.name))
    return undefined
  for (const [key, runtime] of [
    ['pi.event.timestamp.source', 'Pi'],
    ['opencode.event.timestamp.source', 'OpenCode'],
  ]) {
    const source = attribute(event, key)
    if (source?.kind === 'string' && source.value)
      return {
        label: `Reconstructed ${runtime} annotation`,
        description: `Reconstructed from recorded ${runtime} tool evidence and stored as a parent-span event · timestamp: ${source.value}`,
      }
  }
  return undefined
}
