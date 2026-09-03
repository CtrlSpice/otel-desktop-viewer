import type {
  Attribute,
  EventData,
  LinkData,
  ResourceData,
  ScopeData,
  SpanData,
  SpanNode,
  TraceData,
} from '../src/types/api-types'
import { ARM_C_FLAT_FORMAT, ARM_C_FLAT_PATH } from './api-types'

const TRACE_ID_PATTERN = /^[0-9a-f]{32}$/
const SPAN_ID_PATTERN = /^[0-9a-f]{16}$/
const DECIMAL_PATTERN = /^(0|[1-9][0-9]*)$/

export type DecodedFlatTrace = {
  traceID: string
  spans: SpanData[]
}

type SalvagePlacement = {
  spanData: SpanData
  depth: number
}

function requireRecord(value: unknown, path: string): Record<string, unknown> {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) {
    throw new TypeError(`${path} must be an object`)
  }
  return value as Record<string, unknown>
}

function requireExactKeys(
  value: Record<string, unknown>,
  expected: readonly string[],
  path: string
): void {
  const actual = Object.keys(value).sort()
  const wanted = [...expected].sort()
  if (
    actual.length !== wanted.length ||
    actual.some((key, index) => key !== wanted[index])
  ) {
    throw new TypeError(
      `${path} must contain exactly [${wanted.join(', ')}]; got ` +
        `[${actual.join(', ')}]`
    )
  }
}

function requireArray(value: unknown, path: string): unknown[] {
  if (!Array.isArray(value)) throw new TypeError(`${path} must be an array`)
  return value
}

function requireString(value: unknown, path: string): string {
  if (typeof value !== 'string') throw new TypeError(`${path} must be a string`)
  return value
}

function requireNonemptyString(value: unknown, path: string): string {
  const result = requireString(value, path)
  if (result.length === 0) throw new TypeError(`${path} must not be empty`)
  return result
}

function requireSafeInteger(value: unknown, path: string): number {
  if (typeof value !== 'number' || !Number.isSafeInteger(value) || value < 0) {
    throw new TypeError(`${path} must be a nonnegative safe integer`)
  }
  return value
}

function requireHexID(
  value: unknown,
  pattern: RegExp,
  byteLength: number,
  path: string
): string {
  const result = requireString(value, path)
  if (!pattern.test(result) || /^0+$/.test(result)) {
    throw new TypeError(
      `${path} must be ${byteLength} nonzero lowercase hexadecimal bytes`
    )
  }
  return result
}

function requireTraceID(value: unknown, path: string): string {
  return requireHexID(value, TRACE_ID_PATTERN, 16, path)
}

function requireSpanID(value: unknown, path: string): string {
  return requireHexID(value, SPAN_ID_PATTERN, 8, path)
}

function requireNullableSpanID(value: unknown, path: string): string | null {
  if (value === null) return null
  return requireSpanID(value, path)
}

function requireTimestamp(value: unknown, path: string): bigint {
  const raw = requireString(value, path)
  if (!DECIMAL_PATTERN.test(raw)) {
    throw new TypeError(`${path} must be an unsigned decimal string`)
  }
  return BigInt(raw)
}

function decodeAttributes(value: unknown, path: string): Attribute[] {
  return requireArray(value, path).map((candidate, index) => {
    const attributePath = `${path}[${index}]`
    const attribute = requireRecord(candidate, attributePath)
    requireExactKeys(attribute, ['key', 'type', 'value'], attributePath)
    return {
      key: requireString(attribute.key, `${attributePath}.key`),
      type: requireString(attribute.type, `${attributePath}.type`),
      value: requireString(attribute.value, `${attributePath}.value`),
    }
  })
}

function decodeResource(value: unknown, path: string): ResourceData {
  const resource = requireRecord(value, path)
  requireExactKeys(resource, ['attributes', 'droppedAttributesCount'], path)
  return {
    attributes: decodeAttributes(resource.attributes, `${path}.attributes`),
    droppedAttributesCount: requireSafeInteger(
      resource.droppedAttributesCount,
      `${path}.droppedAttributesCount`
    ),
  }
}

function decodeScope(value: unknown, path: string): ScopeData {
  const scope = requireRecord(value, path)
  requireExactKeys(
    scope,
    ['attributes', 'droppedAttributesCount', 'name', 'version'],
    path
  )
  return {
    name: requireString(scope.name, `${path}.name`),
    version: requireString(scope.version, `${path}.version`),
    attributes: decodeAttributes(scope.attributes, `${path}.attributes`),
    droppedAttributesCount: requireSafeInteger(
      scope.droppedAttributesCount,
      `${path}.droppedAttributesCount`
    ),
  }
}

function decodeReferenceMap<T>(
  value: unknown,
  path: string,
  decode: (candidate: unknown, candidatePath: string) => T
): Map<string, T> {
  const source = requireRecord(value, path)
  const result = new Map<string, T>()
  for (const [key, candidate] of Object.entries(source)) {
    if (!DECIMAL_PATTERN.test(key)) {
      throw new TypeError(`${path} key ${JSON.stringify(key)} is not decimal`)
    }
    result.set(key, decode(candidate, `${path}.${key}`))
  }
  return result
}

function decodeEvent(value: unknown, path: string): EventData {
  const event = requireRecord(value, path)
  requireExactKeys(
    event,
    ['attributes', 'droppedAttributesCount', 'name', 'timestamp'],
    path
  )
  return {
    name: requireString(event.name, `${path}.name`),
    timestamp: requireTimestamp(event.timestamp, `${path}.timestamp`),
    attributes: decodeAttributes(event.attributes, `${path}.attributes`),
    droppedAttributesCount: requireSafeInteger(
      event.droppedAttributesCount,
      `${path}.droppedAttributesCount`
    ),
  }
}

function decodeEvents(value: unknown, path: string): EventData[] {
  const events = requireArray(value, path).map((candidate, index) =>
    decodeEvent(candidate, `${path}[${index}]`)
  )
  for (let index = 1; index < events.length; index++) {
    if (events[index]!.timestamp < events[index - 1]!.timestamp) {
      throw new TypeError(`${path} timestamps must be nondecreasing`)
    }
  }
  return events
}

function decodeLink(value: unknown, path: string): LinkData {
  const link = requireRecord(value, path)
  requireExactKeys(
    link,
    [
      'attributes',
      'droppedAttributesCount',
      'flags',
      'spanID',
      'traceID',
      'traceState',
    ],
    path
  )
  return {
    traceID: requireTraceID(link.traceID, `${path}.traceID`),
    spanID: requireSpanID(link.spanID, `${path}.spanID`),
    traceState: requireString(link.traceState, `${path}.traceState`),
    flags: requireSafeInteger(link.flags, `${path}.flags`),
    attributes: decodeAttributes(link.attributes, `${path}.attributes`),
    droppedAttributesCount: requireSafeInteger(
      link.droppedAttributesCount,
      `${path}.droppedAttributesCount`
    ),
  }
}

function decodeRow(
  value: unknown,
  path: string,
  traceID: string,
  resources: ReadonlyMap<string, ResourceData>,
  scopes: ReadonlyMap<string, ScopeData>
): SpanData {
  const row = requireRecord(value, path)
  requireExactKeys(
    row,
    [
      'attributes',
      'droppedAttributesCount',
      'droppedEventsCount',
      'droppedLinksCount',
      'endTime',
      'events',
      'flags',
      'kind',
      'links',
      'name',
      'parentSpanID',
      'resourceRef',
      'scopeRef',
      'spanID',
      'startTime',
      'statusCode',
      'statusMessage',
      'traceState',
    ],
    path
  )

  const resourceRef = requireNonemptyString(
    row.resourceRef,
    `${path}.resourceRef`
  )
  const scopeRef = requireNonemptyString(row.scopeRef, `${path}.scopeRef`)
  const resource = resources.get(resourceRef)
  if (!resource) throw new TypeError(`${path}.resourceRef is not defined`)
  const scope = scopes.get(scopeRef)
  if (!scope) throw new TypeError(`${path}.scopeRef is not defined`)

  const startTime = requireTimestamp(row.startTime, `${path}.startTime`)
  const endTime = requireTimestamp(row.endTime, `${path}.endTime`)
  if (endTime < startTime) {
    throw new TypeError(`${path}.endTime must not precede startTime`)
  }

  return {
    traceID,
    traceState: requireString(row.traceState, `${path}.traceState`),
    spanID: requireSpanID(row.spanID, `${path}.spanID`),
    parentSpanID: requireNullableSpanID(
      row.parentSpanID,
      `${path}.parentSpanID`
    ),
    flags: requireSafeInteger(row.flags, `${path}.flags`),
    name: requireString(row.name, `${path}.name`),
    kind: requireString(row.kind, `${path}.kind`),
    startTime,
    endTime,
    attributes: decodeAttributes(row.attributes, `${path}.attributes`),
    events: decodeEvents(row.events, `${path}.events`),
    links: requireArray(row.links, `${path}.links`).map((candidate, index) =>
      decodeLink(candidate, `${path}.links[${index}]`)
    ),
    resource,
    scope,
    droppedAttributesCount: requireSafeInteger(
      row.droppedAttributesCount,
      `${path}.droppedAttributesCount`
    ),
    droppedEventsCount: requireSafeInteger(
      row.droppedEventsCount,
      `${path}.droppedEventsCount`
    ),
    droppedLinksCount: requireSafeInteger(
      row.droppedLinksCount,
      `${path}.droppedLinksCount`
    ),
    statusCode: requireString(row.statusCode, `${path}.statusCode`),
    statusMessage: requireString(row.statusMessage, `${path}.statusMessage`),
  }
}

export function decodeArmCFlatTrace(
  value: unknown,
  expectedTraceID?: string
): DecodedFlatTrace {
  const trace = requireRecord(value, 'flatTrace')
  requireExactKeys(
    trace,
    ['format', 'resources', 'rows', 'scopes', 'traceID'],
    'flatTrace'
  )
  if (trace.format !== ARM_C_FLAT_FORMAT) {
    throw new TypeError(
      `flatTrace.format must be ${JSON.stringify(ARM_C_FLAT_FORMAT)}`
    )
  }
  const traceID = requireTraceID(trace.traceID, 'flatTrace.traceID')
  if (expectedTraceID !== undefined && traceID !== expectedTraceID) {
    throw new TypeError(
      `flatTrace.traceID ${traceID} does not match requested ${expectedTraceID}`
    )
  }

  const resources = decodeReferenceMap(
    trace.resources,
    'flatTrace.resources',
    decodeResource
  )
  const scopes = decodeReferenceMap(
    trace.scopes,
    'flatTrace.scopes',
    decodeScope
  )
  const spans = requireArray(trace.rows, 'flatTrace.rows').map(
    (candidate, index) =>
      decodeRow(
        candidate,
        `flatTrace.rows[${index}]`,
        traceID,
        resources,
        scopes
      )
  )
  return { traceID, spans }
}

function compareStart(left: SpanData, right: SpanData): number {
  if (left.startTime < right.startTime) return -1
  if (left.startTime > right.startTime) return 1
  return 0
}

function compareStartThenID(left: SpanData, right: SpanData): number {
  const startOrder = compareStart(left, right)
  if (startOrder !== 0) return startOrder
  if (left.spanID < right.spanID) return -1
  if (left.spanID > right.spanID) return 1
  return 0
}

function sortUnambiguous(
  spans: readonly SpanData[],
  description: string
): SpanData[] {
  const sorted = [...spans].sort(compareStart)
  for (let index = 1; index < sorted.length; index++) {
    if (sorted[index]!.startTime === sorted[index - 1]!.startTime) {
      throw new TypeError(
        `${description} has ambiguous equal startTime values for ` +
          `${sorted[index - 1]!.spanID} and ${sorted[index]!.spanID}`
      )
    }
  }
  return sorted
}

export function buildArmCTraceData(flat: DecodedFlatTrace): TraceData {
  const byID = new Map<string, SpanData>()
  for (const span of flat.spans) {
    if (span.traceID !== flat.traceID) {
      throw new TypeError(`span ${span.spanID} has a different traceID`)
    }
    if (byID.has(span.spanID)) {
      throw new TypeError(`duplicate spanID ${span.spanID}`)
    }
    byID.set(span.spanID, span)
  }

  const children = new Map<string, SpanData[]>()
  const genuineRoots: SpanData[] = []
  const orphans: SpanData[] = []
  for (const span of flat.spans) {
    if (span.parentSpanID === null) {
      genuineRoots.push(span)
      continue
    }
    if (!byID.has(span.parentSpanID)) {
      orphans.push(span)
      continue
    }
    const siblings = children.get(span.parentSpanID) ?? []
    siblings.push(span)
    children.set(span.parentSpanID, siblings)
  }

  const sortedChildren = new Map<string, SpanData[]>()
  for (const [parentID, siblings] of children) {
    sortedChildren.set(
      parentID,
      sortUnambiguous(siblings, `children of ${parentID}`)
    )
  }

  const result: SpanNode[] = []
  const emitted = new Set<string>()
  const healthyRoots = [
    ...sortUnambiguous(genuineRoots, 'genuine roots'),
    ...sortUnambiguous(orphans, 'promoted orphans'),
  ]
  for (const root of healthyRoots) {
    const stack = [{ spanData: root, depth: 0 }]
    while (stack.length > 0) {
      const current = stack.pop()!
      if (emitted.has(current.spanData.spanID)) continue
      emitted.add(current.spanData.spanID)
      result.push({
        spanData: current.spanData,
        depth: current.depth,
        matched: true,
      })

      const descendants = sortedChildren.get(current.spanData.spanID) ?? []
      for (let index = descendants.length - 1; index >= 0; index--) {
        stack.push({ spanData: descendants[index]!, depth: current.depth + 1 })
      }
    }
  }

  const stranded = flat.spans.filter(span => !emitted.has(span.spanID))
  const strandedIDs = new Set(stranded.map(span => span.spanID))
  const placements: SalvagePlacement[] = []
  for (const seed of [...stranded].sort(compareStartThenID)) {
    if (emitted.has(seed.spanID)) continue

    const queue: SalvagePlacement[] = [{ spanData: seed, depth: 0 }]
    const visited = new Set<string>()
    for (let cursor = 0; cursor < queue.length; cursor++) {
      const current = queue[cursor]!
      if (visited.has(current.spanData.spanID)) continue
      visited.add(current.spanData.spanID)
      if (emitted.has(current.spanData.spanID)) continue

      emitted.add(current.spanData.spanID)
      placements.push(current)
      const descendants = (children.get(current.spanData.spanID) ?? [])
        .filter(span => strandedIDs.has(span.spanID))
        .sort(compareStartThenID)
      for (const descendant of descendants) {
        queue.push({ spanData: descendant, depth: current.depth + 1 })
      }
    }
  }

  const salvagedIDs = new Set(placements.map(item => item.spanData.spanID))
  for (const placement of placements) {
    const parentID = placement.spanData.parentSpanID
    result.push({
      spanData: placement.spanData,
      depth: placement.depth,
      matched: true,
      salvaged: true,
      cyclePoint:
        placement.depth === 0 && parentID !== null && salvagedIDs.has(parentID),
    })
  }

  return {
    traceID: flat.traceID,
    unplacedSpanCount: flat.spans.length - result.length,
    spans: result,
  }
}

export async function loadArmCTrace(
  traceID: string,
  signal?: AbortSignal
): Promise<TraceData> {
  const response = await fetch(ARM_C_FLAT_PATH, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ traceID }),
    signal,
  })
  if (!response.ok) {
    throw new Error(`Arm C HTTP error: status ${response.status}`)
  }
  const wire: unknown = await response.json()
  return buildArmCTraceData(decodeArmCFlatTrace(wire, traceID))
}
