import type { TraceData } from '../src/types/api-types'

export const TRACE_WATERFALL_SEMANTIC_FORMAT =
  'odv.trace-waterfall.semantic.v1+jcs' as const
export const TRACE_WATERFALL_SEMANTIC_MODEL_FORMAT =
  'odv.trace-waterfall.semantic.v1' as const

export type CanonicalAttribute = {
  key: string
  type: string
  value: string
}

export type CanonicalEvent = {
  name: string
  timestamp: string
  attributes: CanonicalAttribute[]
  droppedAttributesCount: number
}

export type CanonicalLink = {
  traceID: string
  spanID: string
  traceState: string
  flags: number
  attributes: CanonicalAttribute[]
  droppedAttributesCount: number
}

export type CanonicalResource = {
  attributes: CanonicalAttribute[]
  droppedAttributesCount: number
}

export type CanonicalScope = {
  name: string
  version: string
  attributes: CanonicalAttribute[]
  droppedAttributesCount: number
}

export type CanonicalSpan = {
  preorderIndex: number
  depth: number
  matched: boolean
  salvaged: boolean
  cyclePoint: boolean
  spanID: string
  parentSpanID: string | null
  traceState: string
  flags: number
  serviceName: string | null
  operationName: string
  kind: string
  startTime: string
  endTime: string
  status: {
    code: string
    message: string
  }
  droppedAttributesCount: number
  droppedEventsCount: number
  droppedLinksCount: number
  attributes: CanonicalAttribute[]
  events: CanonicalEvent[]
  links: CanonicalLink[]
  resource: CanonicalResource
  scope: CanonicalScope
}

export type TraceWaterfallSemanticProjection = {
  format: typeof TRACE_WATERFALL_SEMANTIC_MODEL_FORMAT
  traceID: string
  unplacedSpanCount: number
  spans: CanonicalSpan[]
}

export type TraceWaterfallSemanticHash = {
  format: typeof TRACE_WATERFALL_SEMANTIC_FORMAT
  hash: string
}

const utf8Encoder = new TextEncoder()

function requireRecord(value: unknown, path: string): Record<string, unknown> {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) {
    throw new TypeError(`${path} must be an object`)
  }
  return value as Record<string, unknown>
}

function requireArray(value: unknown, path: string): unknown[] {
  if (!Array.isArray(value)) {
    throw new TypeError(`${path} must be an array`)
  }
  return value
}

function requireString(value: unknown, path: string): string {
  if (typeof value !== 'string') {
    throw new TypeError(`${path} must be a string`)
  }
  return value
}

function requireNullableString(value: unknown, path: string): string | null {
  if (value === null) return null
  return requireString(value, path)
}

function requireBoolean(value: unknown, path: string): boolean {
  if (typeof value !== 'boolean') {
    throw new TypeError(`${path} must be a boolean`)
  }
  return value
}

function requireOptionalBoolean(value: unknown, path: string): boolean {
  return value === undefined ? false : requireBoolean(value, path)
}

function requireNonnegativeSafeInteger(value: unknown, path: string): number {
  if (
    typeof value !== 'number' ||
    !Number.isFinite(value) ||
    !Number.isSafeInteger(value) ||
    Object.is(value, -0) ||
    value < 0
  ) {
    throw new TypeError(`${path} must be a nonnegative safe integer`)
  }
  return value
}

function requireTimestamp(value: unknown, path: string): bigint {
  if (typeof value !== 'bigint' || value < 0n) {
    throw new TypeError(`${path} must be a nonnegative bigint`)
  }
  return value
}

function compareBytes(left: Uint8Array, right: Uint8Array): number {
  const sharedLength = Math.min(left.length, right.length)
  for (let index = 0; index < sharedLength; index++) {
    const difference = left[index]! - right[index]!
    if (difference !== 0) return difference
  }
  return left.length - right.length
}

function compareUtf8(left: string, right: string): number {
  return compareBytes(utf8Encoder.encode(left), utf8Encoder.encode(right))
}

function compareUtf16(left: string, right: string): number {
  if (left < right) return -1
  if (left > right) return 1
  return 0
}

function readAttributes(value: unknown, path: string): CanonicalAttribute[] {
  return requireArray(value, path).map((candidate, index) => {
    const attribute = requireRecord(candidate, `${path}[${index}]`)
    return {
      key: requireString(attribute.key, `${path}[${index}].key`),
      type: requireString(attribute.type, `${path}[${index}].type`),
      value: requireString(attribute.value, `${path}[${index}].value`),
    }
  })
}

function sortAttributes(
  attributes: readonly CanonicalAttribute[]
): CanonicalAttribute[] {
  return [...attributes].sort(
    (left, right) =>
      compareUtf8(left.key, right.key) ||
      compareUtf8(left.type, right.type) ||
      compareUtf8(left.value, right.value)
  )
}

function projectAttributes(value: unknown, path: string): CanonicalAttribute[] {
  return sortAttributes(readAttributes(value, path))
}

function sortByCanonicalBytes<T>(values: readonly T[]): T[] {
  return values
    .map((value, index) => ({
      value,
      index,
      bytes: utf8Encoder.encode(canonicalizeJSON(value)),
    }))
    .sort(
      (left, right) =>
        compareBytes(left.bytes, right.bytes) || left.index - right.index
    )
    .map(item => item.value)
}

function projectEvents(value: unknown, path: string): CanonicalEvent[] {
  const source = requireArray(value, path)
  const timestamps: bigint[] = []
  const projected = source.map((candidate, index) => {
    const eventPath = `${path}[${index}]`
    const event = requireRecord(candidate, eventPath)
    const timestamp = requireTimestamp(
      event.timestamp,
      `${eventPath}.timestamp`
    )
    if (index > 0 && timestamp < timestamps[index - 1]!) {
      throw new TypeError(`${path} timestamps must be nondecreasing`)
    }
    timestamps.push(timestamp)
    return {
      name: requireString(event.name, `${eventPath}.name`),
      timestamp: timestamp.toString(),
      attributes: projectAttributes(
        event.attributes,
        `${eventPath}.attributes`
      ),
      droppedAttributesCount: requireNonnegativeSafeInteger(
        event.droppedAttributesCount,
        `${eventPath}.droppedAttributesCount`
      ),
    }
  })

  const events: CanonicalEvent[] = []
  for (let start = 0; start < projected.length;) {
    let end = start + 1
    while (end < projected.length && timestamps[end] === timestamps[start]) {
      end++
    }
    events.push(...sortByCanonicalBytes(projected.slice(start, end)))
    start = end
  }
  return events
}

function projectLinks(value: unknown, path: string): CanonicalLink[] {
  const links = requireArray(value, path).map((candidate, index) => {
    const linkPath = `${path}[${index}]`
    const link = requireRecord(candidate, linkPath)
    return {
      traceID: requireString(link.traceID, `${linkPath}.traceID`),
      spanID: requireString(link.spanID, `${linkPath}.spanID`),
      traceState: requireString(link.traceState, `${linkPath}.traceState`),
      flags: requireNonnegativeSafeInteger(link.flags, `${linkPath}.flags`),
      attributes: projectAttributes(link.attributes, `${linkPath}.attributes`),
      droppedAttributesCount: requireNonnegativeSafeInteger(
        link.droppedAttributesCount,
        `${linkPath}.droppedAttributesCount`
      ),
    }
  })
  return sortByCanonicalBytes(links)
}

function projectResource(
  value: unknown,
  path: string
): { resource: CanonicalResource; serviceName: string | null } {
  const source = requireRecord(value, path)
  const attributes = readAttributes(source.attributes, `${path}.attributes`)
  return {
    serviceName:
      attributes.find(attribute => attribute.key === 'service.name')?.value ??
      null,
    resource: {
      attributes: sortAttributes(attributes),
      droppedAttributesCount: requireNonnegativeSafeInteger(
        source.droppedAttributesCount,
        `${path}.droppedAttributesCount`
      ),
    },
  }
}

function projectScope(value: unknown, path: string): CanonicalScope {
  const source = requireRecord(value, path)
  return {
    name: requireString(source.name, `${path}.name`),
    version: requireString(source.version, `${path}.version`),
    attributes: projectAttributes(source.attributes, `${path}.attributes`),
    droppedAttributesCount: requireNonnegativeSafeInteger(
      source.droppedAttributesCount,
      `${path}.droppedAttributesCount`
    ),
  }
}

function projectSpan(
  value: unknown,
  preorderIndex: number,
  envelopeTraceID: string
): CanonicalSpan {
  const path = `trace.spans[${preorderIndex}]`
  const node = requireRecord(value, path)
  const span = requireRecord(node.spanData, `${path}.spanData`)
  const traceID = requireString(span.traceID, `${path}.spanData.traceID`)
  if (traceID !== envelopeTraceID) {
    throw new TypeError(
      `${path}.spanData.traceID must match trace.traceID exactly`
    )
  }

  const salvaged = requireOptionalBoolean(node.salvaged, `${path}.salvaged`)
  const cyclePoint = requireOptionalBoolean(
    node.cyclePoint,
    `${path}.cyclePoint`
  )
  if (cyclePoint && !salvaged) {
    throw new TypeError(`${path}.cyclePoint requires salvaged`)
  }

  if (span.resource === undefined || span.resource === null) {
    throw new TypeError(`${path}.spanData.resource must exist`)
  }
  if (span.scope === undefined || span.scope === null) {
    throw new TypeError(`${path}.spanData.scope must exist`)
  }
  const { resource, serviceName } = projectResource(
    span.resource,
    `${path}.spanData.resource`
  )
  const startTime = requireTimestamp(
    span.startTime,
    `${path}.spanData.startTime`
  )
  const endTime = requireTimestamp(span.endTime, `${path}.spanData.endTime`)
  if (endTime < startTime) {
    throw new TypeError(`${path}.spanData.endTime must not be before startTime`)
  }

  return {
    preorderIndex,
    depth: requireNonnegativeSafeInteger(node.depth, `${path}.depth`),
    matched: requireBoolean(node.matched, `${path}.matched`),
    salvaged,
    cyclePoint,
    spanID: requireString(span.spanID, `${path}.spanData.spanID`),
    parentSpanID: requireNullableString(
      span.parentSpanID,
      `${path}.spanData.parentSpanID`
    ),
    traceState: requireString(span.traceState, `${path}.spanData.traceState`),
    flags: requireNonnegativeSafeInteger(span.flags, `${path}.spanData.flags`),
    serviceName,
    operationName: requireString(span.name, `${path}.spanData.name`),
    kind: requireString(span.kind, `${path}.spanData.kind`),
    startTime: startTime.toString(),
    endTime: endTime.toString(),
    status: {
      code: requireString(span.statusCode, `${path}.spanData.statusCode`),
      message: requireString(
        span.statusMessage,
        `${path}.spanData.statusMessage`
      ),
    },
    droppedAttributesCount: requireNonnegativeSafeInteger(
      span.droppedAttributesCount,
      `${path}.spanData.droppedAttributesCount`
    ),
    droppedEventsCount: requireNonnegativeSafeInteger(
      span.droppedEventsCount,
      `${path}.spanData.droppedEventsCount`
    ),
    droppedLinksCount: requireNonnegativeSafeInteger(
      span.droppedLinksCount,
      `${path}.spanData.droppedLinksCount`
    ),
    attributes: projectAttributes(
      span.attributes,
      `${path}.spanData.attributes`
    ),
    events: projectEvents(span.events, `${path}.spanData.events`),
    links: projectLinks(span.links, `${path}.spanData.links`),
    resource,
    scope: projectScope(span.scope, `${path}.spanData.scope`),
  }
}

/**
 * Projects the fully rehydrated TraceData at the boundary where TracesPage
 * passes traceData.spans to WaterfallView. Input arrays and objects are never
 * mutated.
 */
export function projectTraceWaterfall(
  value: TraceData
): TraceWaterfallSemanticProjection {
  const trace = requireRecord(value, 'trace')
  const traceID = requireString(trace.traceID, 'trace.traceID')
  const sourceSpans = requireArray(trace.spans, 'trace.spans')
  const spans = sourceSpans.map((span, index) =>
    projectSpan(span, index, traceID)
  )

  for (let index = 0; index < spans.length; index++) {
    const depth = spans[index]!.depth
    if (index === 0 && depth !== 0) {
      throw new TypeError('trace.spans[0].depth must be 0')
    }
    if (index > 0 && depth > spans[index - 1]!.depth + 1) {
      throw new TypeError(
        `trace.spans[${index}].depth cannot skip a preorder level`
      )
    }
  }

  return {
    format: TRACE_WATERFALL_SEMANTIC_MODEL_FORMAT,
    traceID,
    unplacedSpanCount: requireNonnegativeSafeInteger(
      trace.unplacedSpanCount,
      'trace.unplacedSpanCount'
    ),
    spans,
  }
}

function unsupportedCanonicalValue(path: string, detail: string): never {
  throw new TypeError(`Unsupported canonical JSON value at ${path}: ${detail}`)
}

function hasLoneSurrogate(value: string): boolean {
  for (let index = 0; index < value.length; index++) {
    const codeUnit = value.charCodeAt(index)
    if (codeUnit >= 0xd800 && codeUnit <= 0xdbff) {
      const next = value.charCodeAt(index + 1)
      if (!(next >= 0xdc00 && next <= 0xdfff)) return true
      index++
    } else if (codeUnit >= 0xdc00 && codeUnit <= 0xdfff) {
      return true
    }
  }
  return false
}

function requireWellFormedString(
  value: string,
  path: string,
  description: string
): void {
  if (hasLoneSurrogate(value)) {
    unsupportedCanonicalValue(
      path,
      `${description} contains a lone UTF-16 surrogate`
    )
  }
}

function serializeCanonicalJSON(
  value: unknown,
  path: string,
  ancestors: Set<object>
): string {
  if (value === null) return 'null'

  switch (typeof value) {
    case 'string':
      requireWellFormedString(value, path, 'string')
      return JSON.stringify(value)
    case 'boolean':
      return value ? 'true' : 'false'
    case 'number':
      if (!Number.isFinite(value)) {
        return unsupportedCanonicalValue(path, 'number must be finite')
      }
      return JSON.stringify(value)
    case 'object': {
      if (ancestors.has(value)) {
        return unsupportedCanonicalValue(path, 'cyclic object')
      }
      if (Object.getOwnPropertySymbols(value).length > 0) {
        return unsupportedCanonicalValue(path, 'symbol keys are not permitted')
      }

      ancestors.add(value)
      try {
        if (Array.isArray(value)) {
          for (let index = 0; index < value.length; index++) {
            if (!Object.prototype.hasOwnProperty.call(value, index)) {
              return unsupportedCanonicalValue(path, 'sparse array')
            }
          }
          for (const key of Object.keys(value)) {
            const index = Number(key)
            if (
              !Number.isInteger(index) ||
              index < 0 ||
              index >= value.length ||
              String(index) !== key
            ) {
              return unsupportedCanonicalValue(path, 'non-index array property')
            }
          }
          return `[${value
            .map((item, index) =>
              serializeCanonicalJSON(item, `${path}[${index}]`, ancestors)
            )
            .join(',')}]`
        }

        const prototype = Object.getPrototypeOf(value)
        if (prototype !== Object.prototype && prototype !== null) {
          return unsupportedCanonicalValue(path, 'non-plain object')
        }
        const record = value as Record<string, unknown>
        const keys = Object.keys(record)
        for (const key of keys) {
          requireWellFormedString(key, path, 'object key')
        }
        keys.sort(compareUtf16)
        return `{${keys
          .map(
            key =>
              `${JSON.stringify(key)}:${serializeCanonicalJSON(
                record[key],
                `${path}.${key}`,
                ancestors
              )}`
          )
          .join(',')}}`
      } finally {
        ancestors.delete(value)
      }
    }
    default:
      return unsupportedCanonicalValue(path, typeof value)
  }
}

/** RFC 8785 JSON canonicalization for values representable as JSON. */
export function canonicalizeJSON(value: unknown): string {
  return serializeCanonicalJSON(value, '$', new Set())
}

/**
 * Hashes a completed semantic projection. Call this only after a benchmark's
 * measured interval has stopped.
 */
export async function hashTraceWaterfallProjection(
  projection: TraceWaterfallSemanticProjection
): Promise<TraceWaterfallSemanticHash> {
  if (!globalThis.crypto?.subtle) {
    throw new Error('Web Crypto SHA-256 is unavailable')
  }
  const canonicalBytes = utf8Encoder.encode(canonicalizeJSON(projection))
  const digest = await globalThis.crypto.subtle.digest(
    'SHA-256',
    canonicalBytes
  )
  const hash = Array.from(new Uint8Array(digest), byte =>
    byte.toString(16).padStart(2, '0')
  ).join('')
  return { format: TRACE_WATERFALL_SEMANTIC_FORMAT, hash }
}
