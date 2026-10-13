import { JSONParser, TokenType } from '@streamparser/json'

export const OTLP_REQUEST_BYTES = 20 * 1024 * 1024
export type ImportSignal = 'traces' | 'logs' | 'metrics'

export const resourceKeys = {
  traces: 'resourceSpans',
  logs: 'resourceLogs',
  metrics: 'resourceMetrics',
} as const

const scopeKeys = {
  traces: 'scopeSpans',
  logs: 'scopeLogs',
  metrics: 'scopeMetrics',
} as const

const recordKeys = {
  traces: 'spans',
  logs: 'logRecords',
  metrics: 'metrics',
} as const

export type FileRange = { start: number; end: number }

/** Only splittable OTLP containers have children; attributes/bodies stay opaque. */
export type ResourceRange = FileRange & { collections: CollectionRange[] }
type CollectionRange = FileRange & {
  items: ResourceRange[]
  splittable: boolean
}

export type ScanIssue = { reason: string; offset: number }
export type ImportPlan = {
  traces: ResourceRange[]
  logs: ResourceRange[]
  metrics: ResourceRange[]
  profiles: ScanIssue[]
  invalid: ScanIssue[]
}

type Role =
  | 'root'
  | 'resources'
  | 'resource'
  | 'scopes'
  | 'scope'
  | 'records'
  | 'metric'
  | 'metricData'
  | 'points'
  | 'leaf'
  | 'opaque'

type Frame = {
  type: 'object' | 'array'
  role: Role
  key: string
  expectingKey: boolean
  signal?: ImportSignal
  node?: ResourceRange
  collection?: CollectionRange
}

function wrapperSignal(key: string): ImportSignal | undefined {
  switch (key) {
    case 'resourceSpans':
      return 'traces'
    case 'resourceLogs':
      return 'logs'
    case 'resourceMetrics':
      return 'metrics'
  }
}

function isMetricData(key: string): boolean {
  return [
    'gauge',
    'sum',
    'histogram',
    'exponentialHistogram',
    'summary',
  ].includes(key)
}

/** One syntax-validation pass; retained data is byte ranges, never telemetry values. */
export async function scanOTLPFile(
  file: Blob,
  signal?: AbortSignal
): Promise<ImportPlan> {
  const plan: ImportPlan = {
    traces: [],
    logs: [],
    metrics: [],
    profiles: [],
    invalid: [],
  }
  // The tokenizer accepts UTF-8 BOMs but reports offsets after the BOM.
  const prefix = new Uint8Array(await file.slice(0, 3).arrayBuffer())
  const byteBase =
    prefix[0] === 0xef && prefix[1] === 0xbb && prefix[2] === 0xbf ? 3 : 0
  const parser = new JSONParser({
    paths: [],
    keepStack: false,
    separator: '\n',
  })
  const frames: Frame[] = []
  let wrappers = 0

  parser.onToken = ({ token, value, offset }) => {
    const position = offset + byteBase
    const parent = frames.at(-1)
    if (token === TokenType.SEPARATOR) return
    if (
      token === TokenType.STRING &&
      parent?.type === 'object' &&
      parent.expectingKey
    ) {
      // The library emits string values for STRING tokens.
      parent.key = value as string
      return
    }
    if (token === TokenType.COLON) {
      if (parent) parent.expectingKey = false
      return
    }
    if (token === TokenType.COMMA) {
      if (parent?.type === 'object') parent.expectingKey = true
      return
    }
    if (token === TokenType.RIGHT_BRACE || token === TokenType.RIGHT_BRACKET) {
      const closed = frames.pop()
      if (closed?.node && closed.role !== 'metricData')
        closed.node.end = position + 1
      if (closed?.collection) closed.collection.end = position + 1
      return
    }

    const object = token === TokenType.LEFT_BRACE
    const array = token === TokenType.LEFT_BRACKET
    let role: Role = 'opaque'
    let node: ResourceRange | undefined
    let collection: CollectionRange | undefined
    let currentSignal = parent?.signal

    if (!parent) {
      if (!object) throw new Error('OTLP input must contain JSON objects.')
      role = 'root'
    } else if (parent.role === 'root') {
      wrappers++
      currentSignal = wrapperSignal(parent.key)
      if (parent.key === 'resourceProfiles') {
        plan.profiles.push({
          reason: 'Profiles support coming soon',
          offset: position,
        })
      } else if (!currentSignal) {
        plan.invalid.push({
          reason: `Invalid OTLP wrapper: ${parent.key}.`,
          offset: position,
        })
      } else if (!array) {
        plan.invalid.push({
          reason: `${parent.key} must be an array.`,
          offset: position,
        })
      } else {
        role = 'resources'
      }
    } else if (parent.role === 'resources' && currentSignal) {
      if (object) {
        role = 'resource'
        node = { start: position, end: position, collections: [] }
        plan[currentSignal].push(node)
      } else {
        plan.invalid.push({
          reason: `${resourceKeys[currentSignal]} must contain resource objects.`,
          offset: position,
        })
      }
    } else if (parent.type === 'array' && parent.collection) {
      if (object) {
        role =
          parent.role === 'scopes'
            ? 'scope'
            : currentSignal === 'metrics' && parent.role === 'records'
              ? 'metric'
              : 'leaf'
        node = { start: position, end: position, collections: [] }
        parent.collection.items.push(node)
      } else {
        // Leave malformed OTLP intact for receiver validation; never omit it when splitting.
        parent.collection.splittable = false
      }
    } else if (parent.node && currentSignal) {
      if (
        array &&
        parent.role === 'resource' &&
        parent.key === scopeKeys[currentSignal]
      )
        role = 'scopes'
      if (
        array &&
        parent.role === 'scope' &&
        parent.key === recordKeys[currentSignal]
      )
        role = 'records'
      if (array && parent.role === 'metricData' && parent.key === 'dataPoints')
        role = 'points'
      if (object && parent.role === 'metric' && isMetricData(parent.key)) {
        role = 'metricData'
        node = parent.node
      }
      if (role === 'scopes' || role === 'records' || role === 'points') {
        collection = {
          start: position,
          end: position,
          items: [],
          splittable: true,
        }
        parent.node.collections.push(collection)
      }
    }

    if (object || array) {
      frames.push({
        type: object ? 'object' : 'array',
        role,
        key: '',
        expectingKey: object,
        signal: currentSignal,
        node,
        collection,
      })
    }
  }

  const reader = file.stream().getReader()
  try {
    while (true) {
      signal?.throwIfAborted()
      const next = await reader.read()
      if (next.done) break
      parser.write(next.value)
    }
    parser.end()
    if (wrappers === 0)
      plan.invalid.push({
        reason: 'No OTLP resource wrappers found.',
        offset: 0,
      })
    return plan
  } finally {
    await reader.cancel()
    reader.releaseLock()
  }
}

export type ImportBatch = { body: Blob } | { issue: ScanIssue }

/** Split only known collection boundaries, copying all surrounding metadata bytes. */
function* splitResource(
  file: Blob,
  resource: ResourceRange,
  budget: number
): Generator<Blob | ScanIssue> {
  if (resource.end - resource.start <= budget) {
    yield file.slice(resource.start, resource.end)
    return
  }
  if (resource.collections.length !== 1) {
    yield {
      reason:
        'An individual OTLP record or its metadata exceeds the 20 MiB request limit.',
      offset: resource.start,
    }
    return
  }
  const list = resource.collections[0]
  if (!list.splittable) {
    yield {
      reason: 'Invalid OTLP collection cannot be split into requests.',
      offset: list.start,
    }
    return
  }
  const before = file.slice(resource.start, list.start + 1)
  const after = file.slice(list.end - 1, resource.end)
  const innerBudget = budget - before.size - after.size
  if (innerBudget <= 0 || list.items.length === 0) {
    yield {
      reason:
        'OTLP resource or scope metadata exceeds the 20 MiB request limit.',
      offset: resource.start,
    }
    return
  }
  let parts: BlobPart[] = []
  let size = 0
  for (const item of list.items) {
    for (const piece of splitResource(file, item, innerBudget)) {
      if (!(piece instanceof Blob)) {
        yield piece
        continue
      }
      const separator = parts.length ? 1 : 0
      if (size + separator + piece.size > innerBudget) {
        yield new Blob([before, ...parts, after])
        parts = []
        size = 0
      }
      if (parts.length) {
        parts.push(',')
        size++
      }
      parts.push(piece)
      size += piece.size
    }
  }
  if (parts.length) yield new Blob([before, ...parts, after])
}

export function* batchOTLPFile(
  file: Blob,
  signal: ImportSignal,
  resources: readonly ResourceRange[],
  maximumBytes = OTLP_REQUEST_BYTES
): Generator<ImportBatch> {
  const opening = `{"${resourceKeys[signal]}":[`
  const closing = ']}'
  const budget = maximumBytes - opening.length - closing.length
  let parts: BlobPart[] = []
  let size = 0
  for (const resource of resources) {
    for (const piece of splitResource(file, resource, budget)) {
      if (!(piece instanceof Blob)) {
        yield { issue: piece }
        continue
      }
      if (size + (parts.length ? 1 : 0) + piece.size > budget) {
        yield {
          body: new Blob([opening, ...parts, closing], {
            type: 'application/json',
          }),
        }
        parts = []
        size = 0
      }
      if (parts.length) {
        parts.push(',')
        size++
      }
      parts.push(piece)
      size += piece.size
    }
  }
  if (parts.length)
    yield {
      body: new Blob([opening, ...parts, closing], {
        type: 'application/json',
      }),
    }
}
