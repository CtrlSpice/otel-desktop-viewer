import type { Page, Route, Request } from '@playwright/test'
import {
  Bool,
  DataType,
  DenseUnion,
  Field,
  Float64,
  Int64,
  List,
  Map_,
  Null,
  Struct,
  Table,
  RecordBatch,
  Schema,
  Uint64,
  Utf8,
  Binary,
  makeData,
  tableToIPC,
  vectorFromArray,
  type Data,
} from 'apache-arrow'
import type { JsonAttributeValue } from '../src/types/wire-types'

type Value =
  | null
  | boolean
  | string
  | number
  | bigint
  | Uint8Array
  | Value[]
  | Map<string, Value>
  | { [key: string]: Value }

function kind(value: Value): string {
  if (value === null) return 'null'
  if (value instanceof Uint8Array) return 'bytes'
  if (value instanceof Map) return 'map'
  if (Array.isArray(value)) return 'list'
  return typeof value
}

/** Fixture writer only. Browser production reads native Arrow, never these objects. */
function column(values: Value[]): Data {
  const kinds = [
    ...new Set(values.flatMap(v => (v === null ? [] : [kind(v)]))),
  ].sort()
  if (kinds.length > 1) {
    const groups = kinds.map(k =>
      values.filter(v => v !== null && kind(v) === k)
    )
    if (values.includes(null)) {
      kinds.push('null')
      groups.push(values.filter(v => v === null))
    }
    const children = groups.map(column)
    const offsets = kinds.map(() => 0)
    const ids = values.map(v => kinds.indexOf(kind(v)))
    const type = new DenseUnion(
      kinds.map((_, i) => i),
      children.map((c, i) => new Field(kinds[i], c.type, true))
    )
    return makeData({
      type,
      length: values.length,
      children,
      typeIds: Int8Array.from(ids),
      valueOffsets: Int32Array.from(ids.map(id => offsets[id]++)),
    })
  }
  const category = kinds[0] ?? 'null'
  const nullCount = values.filter(v => v === null).length
  const nullBitmap = new Uint8Array(Math.ceil(values.length / 8))
  values.forEach((v, i) => {
    if (v !== null) nullBitmap[i >> 3] |= 1 << (i % 8)
  })
  if (category === 'object') {
    const objects = values as ({ [key: string]: Value } | null)[]
    const keys = [...new Set(objects.flatMap(v => Object.keys(v ?? {})))]
    const children = keys.map(key => column(objects.map(v => v?.[key] ?? null)))
    const type = new Struct(
      children.map((child, i) => new Field(keys[i], child.type, true))
    )
    return makeData({
      type,
      length: values.length,
      children,
      nullCount,
      nullBitmap,
    })
  }
  if (category === 'list' || category === 'map') {
    const lists = values.map(v =>
      v === null
        ? []
        : v instanceof Map
          ? [...v].map(([key, value]) => ({ key, value }))
          : (v as Value[])
    )
    let size = 0
    const offsets = [0, ...lists.map(v => (size += v.length))]
    const flat = lists.flat()
    // Empty record lists still have a Struct element type, like native datasets.
    const child = flat.length
      ? column(flat)
      : makeData({ type: new Struct([]), length: 0, children: [] })
    if (category === 'map') {
      const entries =
        DataType.isStruct(child.type) && child.type.children.length
          ? child
          : column([{ key: '', value: null }]).slice(0, 0)
      const type = new Map_(new Field('entries', entries.type as Struct, false))
      return makeData({
        type,
        length: values.length,
        child: entries,
        valueOffsets: Int32Array.from(offsets),
        nullCount,
        nullBitmap,
      })
    }
    return makeData({
      type: new List(new Field('item', child.type, true)),
      length: values.length,
      child,
      valueOffsets: Int32Array.from(offsets),
      nullCount,
      nullBitmap,
    })
  }
  const type =
    category === 'string'
      ? new Utf8()
      : category === 'boolean'
        ? new Bool()
        : category === 'number'
          ? new Float64()
          : category === 'bigint'
            ? values.some(v => v !== null && (v as bigint) < 0n)
              ? new Int64()
              : new Uint64()
            : category === 'bytes'
              ? new Binary()
              : new Null()
  return vectorFromArray(values, type).data[0]!
}

function value(v: JsonAttributeValue): Value {
  switch (v.kind) {
    case 'empty':
      return null
    case 'int64':
      return BigInt(v.value)
    case 'double': {
      if (typeof v.value === 'number') return v.value
      const bits = new DataView(new ArrayBuffer(8))
      bits.setBigUint64(0, BigInt(v.value))
      return bits.getFloat64(0)
    }
    case 'array':
      return v.value.map(value)
    case 'map':
      return new Map(v.value.map(e => [e.key, value(e.value)]))
    case 'bytes':
      return new Uint8Array(Buffer.from(v.value, 'base64'))
    default:
      return v.value
  }
}

type RecordFixture = Record<string, any>
function attrs(
  list: { key: string; value: JsonAttributeValue }[] = []
): Map<string, Value> {
  return new Map(list.map(a => [a.key, value(a.value)]))
}
function resource(r: RecordFixture = {}): Value {
  return {
    attributes: attrs(r.attributes),
    droppedAttributesCount: r.droppedAttributesCount ?? 0,
    schemaUrl: r.schemaUrl ?? '',
  }
}
function scope(s: RecordFixture = {}): Value {
  return {
    ...(resource(s) as Record<string, Value>),
    name: s.name ?? '',
    version: s.version ?? '',
  }
}
function logSummary(r: RecordFixture) {
  const result = {
    logRef: r.logRef,
    timestamp: BigInt(r.timestamp ?? 0),
    severityText: r.severityText ?? '',
    severityNumber: r.severityNumber ?? 0,
    eventName: r.eventName ?? '',
    serviceName: r.serviceName ?? '',
    body: r.body ? value(r.body) : (r.bodyPreview ?? ''),
  }
  if (r.spanID !== undefined) Object.assign(result, { spanID: r.spanID })
  if (r.columnValues)
    Object.assign(result, {
      columnValues: r.columnValues.map((c: RecordFixture) => ({
        source: c.source,
        kind: c.kind,
        key: c.key,
        present: c.values.length > 0,
        value: c.values.length ? value(c.values[0]) : null,
      })),
    })
  return result
}
function log(r: RecordFixture) {
  return {
    ...logSummary(r),
    observedTimestamp: BigInt(r.observedTimestamp ?? 0),
    traceID: r.traceID ?? null,
    spanID: r.spanID ?? null,
    body: value(r.body),
    flags: r.flags ?? 0,
    droppedAttributesCount: r.droppedAttributesCount ?? 0,
    attributes: attrs(r.attributes),
    resource: resource({ ...r.resource, schemaUrl: r.resourceSchemaURL }),
    scope: scope({ ...r.scope, schemaUrl: r.scopeSchemaURL }),
  }
}

/** Reuse semantic browser fixtures while supplying the new native wire schemas. */
function dataset(method: string, result: any): Table {
  let rows: Record<string, Value>[]
  switch (method) {
    case 'searchTraceSummaries':
      rows = result.map((r: RecordFixture) => {
        const row = {
          traceID: r.traceID,
          hasRootSpan: r.hasRootSpan ?? Boolean(r.rootSpan),
          rootSpan: r.rootSpan ?? null,
          startTime: BigInt(r.startTime ?? 0),
          endTime:
            r.durationNs == null
              ? null
              : BigInt(r.startTime ?? 0) + BigInt(r.durationNs),
          spanCount: BigInt(r.spanCount ?? 0),
          errorCount: BigInt(r.errorCount ?? 0),
          serviceBreakdown: (r.serviceBreakdown ?? []).map(
            (part: RecordFixture) => ({ ...part, other: part.other ?? false })
          ),
        }
        if (r.matchedSpans) Object.assign(row, { matchedSpans: r.matchedSpans })
        return row
      })
      break
    case 'searchMetricSummaries':
      rows = result.map((r: RecordFixture) => ({
        ...r,
        unit: r.unit ?? '',
        description: r.description ?? '',
        lastSeen: r.lastSeen == null ? null : BigInt(r.lastSeen),
        aggregationTemporalityCode: r.aggregationTemporalityCode ?? null,
        isMonotonic: r.isMonotonic ?? null,
        lastValue:
          r.lastValue == null
            ? null
            : value({ kind: 'double', value: r.lastValue }),
      }))
      break
    case 'searchLogSummaries':
    case 'getTraceLogSummaries':
      rows = result.map(logSummary)
      break
    case 'searchLogPage':
      rows = [
        {
          total: BigInt(result.total ?? 0),
          logs: (result.logs ?? []).map(logSummary),
        },
      ]
      break
    case 'getLog':
      rows = [log(result)]
      break
    case 'getSpanLogs':
      rows = (result.logs ?? []).map(log)
      break
    default:
      throw new Error(`Missing native fixture projection for ${method}`)
  }
  const data = rows.length
    ? column(rows)
    : makeData({ type: new Struct([]), length: 0, children: [] })
  return new Table(
    new RecordBatch(new Schema(data.type.children), data as Data<Struct>)
  )
}

export function telemetryRequest(request: Request) {
  const method = new URL(request.url()).pathname.match(
    /^\/api\/data\/(.+)$/
  )?.[1]
  return method
    ? {
        id: 0,
        method: method === 'getSpanLogs' ? 'getSpan' : method,
        params: request.postDataJSON(),
      }
    : request.postDataJSON()
}

export type FixtureRoute = Pick<Route, 'fallback' | 'continue' | 'abort'> & {
  request(): { postDataJSON(): ReturnType<typeof telemetryRequest> }
  fulfill(options: Parameters<Route['fulfill']>[0]): Promise<void>
}

export async function routeTelemetry(
  page: Page,
  handler: (route: FixtureRoute) => Promise<void> | void
) {
  await page.route(/\/(?:rpc|api\/data\/[^/]+)$/, async route => {
    const nativeMethod = new URL(route.request().url()).pathname.match(
      /^\/api\/data\/(.+)$/
    )?.[1]
    if (!nativeMethod) return handler(route)
    return handler({
      request: () => ({
        postDataJSON: () => telemetryRequest(route.request()),
      }),
      fallback: options => route.fallback(options),
      continue: options => route.continue(options),
      abort: code => route.abort(code),
      fulfill: async options => {
        const response = options?.json ?? JSON.parse(String(options?.body))
        if (response.error)
          return route.fulfill({ status: 400, json: response.error })
        const table = dataset(nativeMethod, response.result)
        return route.fulfill({
          status: options?.status ?? 200,
          contentType: 'application/vnd.apache.arrow.stream',
          headers: { 'X-Arrow-Rows': String(table.numRows) },
          body: Buffer.from(tableToIPC(table, 'stream')),
        })
      },
    })
  })
}
