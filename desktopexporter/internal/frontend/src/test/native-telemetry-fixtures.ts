import {
  Bool,
  DataType,
  DenseUnion,
  Decimal,
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
  vectorFromArray,
  type Data,
} from 'apache-arrow'
import type { JsonAttributeValue, JsonTraceData } from '../types/wire-types'

type Value =
  | null
  | boolean
  | string
  | number
  | bigint
  | Uint8Array
  | Value[]
  | Map<string, Value>
  | MapEntries
  | { [key: string]: Value }

class MapEntries {
  constructor(readonly entries: [string, Value][]) {}
}

function kind(value: Value): string {
  if (value === null) return 'null'
  if (value instanceof Uint8Array) return 'bytes'
  if (value instanceof Map || value instanceof MapEntries) return 'map'
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
  if (
    category === 'bigint' &&
    (values.some(
      v =>
        v !== null &&
        ((v as bigint) < -(1n << 63n) || (v as bigint) >= 1n << 64n)
    ) ||
      (values.some(v => v !== null && (v as bigint) < 0n) &&
        values.some(v => v !== null && (v as bigint) >= 1n << 63n)))
  ) {
    const data = new Uint32Array(values.length * 4)
    values.forEach((v, i) => {
      let bits = BigInt.asUintN(128, (v as bigint | null) ?? 0n)
      for (let word = 0; word < 4; word++) {
        data[i * 4 + word] = Number(bits & 0xffffffffn)
        bits >>= 32n
      }
    })
    return makeData({
      type: new Decimal(0, 38, 128),
      data,
      length: values.length,
      nullCount,
      nullBitmap,
    })
  }
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
        : v instanceof MapEntries
          ? v.entries.map(([key, value]) => ({ key, value }))
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
      return new MapEntries(v.value.map(e => [e.key, value(e.value)]))
    case 'bytes':
      return new Uint8Array(Buffer.from(v.value, 'base64'))
    default:
      return v.value
  }
}

type RecordFixture = Record<string, any>
function attrs(
  list: { key: string; value: JsonAttributeValue }[] = []
): MapEntries {
  return new MapEntries(list.map(a => [a.key, value(a.value)]))
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
    bodyPreview: r.bodyPreview ?? '',
    bodyType: r.bodyType ?? r.body?.kind ?? 'string',
  }
  if (r.spanID !== undefined) Object.assign(result, { spanID: r.spanID })
  if (r.columnValues)
    Object.assign(result, {
      columnValues: r.columnValues.map((c: RecordFixture) => ({
        source: c.source,
        kind: c.kind,
        key: c.key,
        present: c.present,
        valueType: c.valueType,
        preview: c.preview,
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
export function telemetryDataset(method: string, result: any): Table {
  let rows: Record<string, Value>[]
  switch (method) {
    case 'getMetricSeriesOrder':
      rows = result.map((seriesRef: string) => ({ seriesRef }))
      break
    case 'getTraceAttributeDefinitions':
    case 'getTraceAttributeDefinitionsByTraceID':
    case 'getLogAttributeDefinitions':
    case 'getMetricAttributeDefinitions':
    case 'searchAttributeMatches':
      rows = result.map((entry: RecordFixture) => {
        const row = {
          name: entry.name,
          attributeScope: entry.attributeScope,
          owner:
            entry.owner ??
            (entry.attributeScope === 'metadata'
              ? 'metric'
              : entry.attributeScope),
          path: (
            entry.path ?? [
              entry.attributeScope === 'metadata' ? 'metadata' : 'attributes',
              entry.name,
            ]
          ).map((part: string | number) => ({
            key: typeof part === 'string' ? part : null,
            index: typeof part === 'number' ? BigInt(part) : null,
          })),
          type: entry.type,
        }
        if (method === 'searchAttributeMatches') {
          return {
            ...row,
            matchCount: entry.matchCount,
            sampleValues: entry.sampleValues.map(value),
          }
        }
        return row
      })
      break
    case 'getFieldValueCompletions':
      rows = result.map((value: string) => ({ value }))
      break
    case 'discoverTelemetry':
      rows = [
        {
          items: result.rows.map(([entry]: [RecordFixture]) =>
            discoveryEntry(entry)
          ),
          truncated: result.truncated ?? false,
          totals: result.totals ?? null,
        },
      ]
      break
    case 'getMetric':
      rows = [
        {
          ...metricIdentity(result),
          series: result.series.map((s: RecordFixture) => ({
            ...s,
            attributes: attrs(s.attributes),
            datapointCount: BigInt(s.datapointCount),
            firstDatapointTimestamp: big(s.firstDatapointTimestamp),
            lastDatapointTimestamp: big(s.lastDatapointTimestamp),
          })),
        },
      ]
      break
    case 'getMetricSeries':
      rows = [
        {
          ...metricIdentity(result),
          seriesRef: result.seriesRef,
          attributes: attrs(result.attributes),
          datapoints: result.datapoints.map(metricPoint),
        },
      ]
      break
    case 'getMetricView':
      rows = [
        {
          ...metricIdentity(result),
          lastSeenNs: big(result.lastSeenNs),
          datapointCount: result.datapointCount ?? 0,
          boundsMismatch: result.boundsMismatch ?? null,
          window: {
            requested: {
              startNs: big(result.window.requested.startNs),
              endNs: big(result.window.requested.endNs),
            },
            effective: {
              startNs: big(result.window.effective.startNs),
              endNs: big(result.window.effective.endNs),
            },
          },
          timeseries: result.timeseries.map((s: RecordFixture) => ({
            seriesRef: s.seriesRef,
            attributes: s.labelPreviews ? attrs([]) : attrs(s.attributes),
            attributesComplete: s.attributesComplete ?? true,
            labelPreviews:
              s.labelPreviews?.map((cell: RecordFixture) => ({
                ...cell,
                kind: cell.kind ?? '',
              })) ?? null,
            resource:
              s.attributesComplete === false ? null : resource(s.resource),
            datapoints: s.datapoints.map(metricPoint),
            stats: stats(s.stats),
            rateStats: stats(s.rateStats),
            datapointCount: s.datapointCount ?? 0,
            lastSeenNs: big(s.lastSeenNs),
            views: s.views?.map(scalarBucket) ?? null,
            sparkline:
              s.sparkline?.map((p: RecordFixture) => ({
                timestamp: BigInt(p.timestamp),
                value: double(p.value),
              })) ?? null,
          })),
        },
      ]
      break
    case 'getMetricAggregateView': {
      if (result === null) {
        rows = []
        break
      }
      const row = {
        aggregate: result.aggregate?.map(metricPoint) ?? null,
        scalarAggregate: result.scalarAggregate
          ? {
              selected: result.scalarAggregate.selected.map(scalarBucket),
              all: result.scalarAggregate.all.map(scalarBucket),
            }
          : null,
      }
      if (result.contribution !== undefined)
        Object.assign(row, {
          contribution: result.contribution?.map(metricPoint) ?? null,
        })
      rows = [row]
      break
    }
    case 'getTraceView':
      rows = [trace(result)]
      break
    case 'getSpan':
      rows = result === null ? [] : [trace(result)]
      break
    case 'getTraceColumnPreviews':
      rows = result
      break
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

function discoveryEntry(entry: RecordFixture) {
  if ('source' in entry)
    return {
      ...entry,
      source: discoveryService(entry.source),
      target: discoveryService(entry.target),
    }
  if ('value' in entry)
    return {
      ...entry,
      value: value(entry.value),
      identity: JSON.stringify(entry.value),
    }
  if ('startTime' in entry)
    return {
      ...entry,
      startTime: BigInt(entry.startTime),
      endTime: BigInt(entry.endTime),
    }
  if ('serviceName' in entry) return discoveryService(entry)
  return entry
}

function discoveryService(entry: RecordFixture) {
  return {
    serviceIdentity: JSON.stringify([
      entry.serviceNamespace,
      entry.serviceName,
    ]),
    serviceNameKind: 'string',
    serviceNamespaceKind: 'string',
    traceCount: 0,
    spanCount: 0,
    errorSpanCount: 0,
    logCount: 0,
    errorLogCount: 0,
    metricCount: 0,
    dataPointCount: 0,
    exemplarCount: 0,
    logSeverityCounts: {
      trace: 0,
      debug: 0,
      info: 0,
      warn: 0,
      error: 0,
      fatal: 0,
      other: 0,
    },
    ...entry,
    lastSeen: big(entry.lastSeen),
  }
}

function traceColumnCatalogue(data: JsonTraceData) {
  const counts = new Map<
    string,
    { source: string; kind: string; key: string; count: number }
  >()
  for (const node of data.spans) {
    for (const source of ['span', 'resource', 'scope'] as const) {
      const attributes =
        source === 'span'
          ? node.spanData.attributes
          : source === 'resource'
            ? data.resources[String(node.spanData.r)].attributes
            : data.scopes[String(node.spanData.s)].attributes
      for (const key of new Set(attributes.map(attribute => attribute.key))) {
        const id = JSON.stringify([source, key])
        const column = counts.get(id)
        if (column) column.count++
        else counts.set(id, { source, kind: 'attribute', key, count: 1 })
      }
    }
  }
  return [...counts.values()]
}

function trace(data: JsonTraceData) {
  const resourceEntries = Object.entries(data.resources)
  const scopeEntries = Object.entries(data.scopes)
  return {
    traceID: data.traceID,
    traceStart: BigInt(data.traceStart),
    unplacedSpanCount: BigInt(data.unplacedSpanCount),
    availableColumns: traceColumnCatalogue(data),
    resources: resourceEntries.map(([, r]) => ({
      ...(resource(r) as Record<string, Value>),
      serviceName:
        r.attributes.find(
          a => a.key === 'service.name' && a.value.kind === 'string'
        )?.value.value ?? '',
    })),
    scopes: scopeEntries.map(([, s]) => scope(s)),
    spans: data.spans.map(node => ({
      depth: node.depth,
      matched: node.matched,
      salvaged: node.salvaged ?? false,
      cyclePoint: node.cyclePoint ?? false,
      spanData: {
        ...node.spanData,
        r:
          resourceEntries.findIndex(
            ([key]) => key === String(node.spanData.r)
          ) + 1,
        s:
          scopeEntries.findIndex(([key]) => key === String(node.spanData.s)) +
          1,
        startTime: BigInt(data.traceStart) + BigInt(node.spanData.start),
        endTime:
          BigInt(data.traceStart) +
          BigInt(node.spanData.start) +
          BigInt(node.spanData.dur),
        attributes: attrs(node.spanData.attributes),
        events: node.spanData.events.map(e => ({
          ...e,
          timestamp: BigInt(e.timestamp),
          attributes: attrs(e.attributes),
          summaryAttributes: attrs(
            e.attributes.filter(a =>
              [
                'gen_ai.tool.name',
                'tool.is_error',
                'pi.event.timestamp.source',
                'opencode.event.timestamp.source',
              ].includes(a.key)
            )
          ),
        })),
        links: node.spanData.links.map(l => ({
          ...l,
          attributes: attrs(l.attributes),
          summary: l.summary ?? { target: null },
        })),
      },
    })),
  }
}

function big(value: string | number | null | undefined) {
  return value == null ? null : BigInt(value)
}
function double(v: string | number | null | undefined): Value {
  return v == null ? null : value({ kind: 'double', value: v })
}
function stats(s: RecordFixture | null | undefined) {
  if (!s) return null
  return {
    ...s,
    min: double(s.min),
    max: double(s.max),
    sum: double(s.sum),
    avg: double(s.avg),
  }
}
function metricIdentity(r: RecordFixture) {
  return {
    metricRef: r.metricRef,
    name: r.name ?? '',
    description: r.description ?? '',
    unit: r.unit ?? '',
    metadata: attrs(r.metadata),
    metricType: r.metricType ?? 'Gauge',
    aggregationTemporalityCode: r.aggregationTemporalityCode ?? null,
    isMonotonic: r.isMonotonic ?? null,
    resource: resource({
      ...r.resource,
      schemaUrl: r.resource?.schemaUrl ?? r.resourceSchemaUrl,
    }),
    scope: scope({
      ...r.scope,
      schemaUrl: r.scope?.schemaUrl ?? r.scopeSchemaUrl,
    }),
  }
}
function metricPoint(r: RecordFixture) {
  return {
    ...r,
    timestamp: BigInt(r.timestamp),
    startTime: BigInt(r.startTime),
    exemplars: (r.exemplars ?? []).map((e: RecordFixture) => ({
      ...e,
      timestamp: BigInt(e.timestamp),
      intValue: big(e.intValue),
      doubleValue: double(e.doubleValue),
      filteredAttributes: attrs(e.filteredAttributes),
    })),
    exemplarCount: r.exemplarCount ?? null,
    intValue: big(r.intValue),
    doubleValue: double(r.doubleValue),
    count: big(r.count),
    sum: double(r.sum),
    min: double(r.min),
    max: double(r.max),
    zeroCount: big(r.zeroCount),
    zeroThreshold: double(r.zeroThreshold),
    bucketCounts: r.bucketCounts?.map(BigInt) ?? null,
    explicitBounds: r.explicitBounds?.map(double) ?? null,
    positiveBucketCounts: r.positiveBucketCounts?.map(BigInt) ?? null,
    negativeBucketCounts: r.negativeBucketCounts?.map(BigInt) ?? null,
    positive: r.positive
      ? {
          offset: r.positive.offset,
          bucketCounts: r.positive.bucketCounts.map(BigInt),
        }
      : null,
    negative: r.negative
      ? {
          offset: r.negative.offset,
          bucketCounts: r.negative.bucketCounts.map(BigInt),
        }
      : null,
    quantiles:
      r.quantiles == null
        ? null
        : new Map(
            Object.entries(r.quantiles).map(([key, v]) => [
              key,
              double(v as number | string | null),
            ])
          ),
    deltaInt:
      typeof r.delta === 'string' && !r.delta.startsWith('0x')
        ? BigInt(r.delta)
        : null,
    deltaDouble:
      typeof r.delta === 'number' ||
      (typeof r.delta === 'string' && r.delta.startsWith('0x'))
        ? double(r.delta)
        : null,
    isReset: r.isReset ?? null,
  }
}
function scalarBucket(r: RecordFixture) {
  return {
    ...r,
    bucketStart: BigInt(r.bucketStart),
    sum: double(r.sum),
    avg: double(r.avg),
    rate: double(r.rate),
    slope: double(r.slope),
  }
}
