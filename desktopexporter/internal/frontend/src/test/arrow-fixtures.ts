import {
  Bool,
  Field,
  Int32,
  Int64,
  List,
  Map_,
  Struct,
  Table,
  Uint64,
  Utf8,
  tableToIPC,
  vectorFromArray,
} from 'apache-arrow'

export function arrowResponse(table: Table): Response {
  return new Response(new Uint8Array(tableToIPC(table, 'stream')), {
    headers: {
      'Content-Type': 'application/vnd.apache.arrow.stream',
      'X-Arrow-Rows': String(table.numRows),
    },
  })
}

export function traceSummaryTable(
  rows: {
    traceID: string
    startTime: bigint
    endTime: bigint | null
    hasRootSpan?: boolean
    rootSpan?: { name: string; serviceName: string } | null
    spanCount?: number
    errorCount?: number
    matchedSpans?: { traceID: string; spanID: string }[] | null
  }[]
): Table {
  const table = new Table({
    traceID: vectorFromArray(
      rows.map(r => r.traceID),
      new Utf8()
    ),
    startTime: vectorFromArray(
      rows.map(r => r.startTime),
      new Uint64()
    ),
    endTime: vectorFromArray(
      rows.map(r => r.endTime),
      new Uint64()
    ),
    hasRootSpan: vectorFromArray(
      rows.map(r => r.hasRootSpan ?? false),
      new Bool()
    ),
    rootSpan: vectorFromArray(
      rows.map(r => r.rootSpan ?? null),
      new Struct([
        new Field('name', new Utf8()),
        new Field('serviceName', new Utf8()),
      ])
    ),
    spanCount: vectorFromArray(
      rows.map(r => BigInt(r.spanCount ?? 0)),
      new Int64()
    ),
    errorCount: vectorFromArray(
      rows.map(r => BigInt(r.errorCount ?? 0)),
      new Int64()
    ),
    serviceBreakdown: vectorFromArray(
      rows.map(() => []),
      new List(
        new Field(
          'item',
          new Struct([
            new Field('key', new Utf8()),
            new Field('label', new Utf8()),
            new Field('count', new Int64()),
            new Field('other', new Bool()),
          ])
        )
      )
    ),
  })
  if (rows.some(r => r.matchedSpans))
    return table.assign(
      new Table({
        matchedSpans: vectorFromArray(
          rows.map(r => r.matchedSpans ?? null),
          new List(
            new Field(
              'item',
              new Struct([
                new Field('traceID', new Utf8()),
                new Field('spanID', new Utf8()),
              ])
            )
          )
        ),
      })
    )
  return table
}

export function logSummaryTable(): Table {
  return new Table({
    logRef: vectorFromArray(['log-1'], new Utf8()),
    timestamp: vectorFromArray([9223372036854775807n], new Uint64()),
    spanID: vectorFromArray([null], new Utf8()),
    severityText: vectorFromArray(['INFO'], new Utf8()),
    severityNumber: vectorFromArray([9], new Int32()),
    serviceName: vectorFromArray(['checkout'], new Utf8()),
    eventName: vectorFromArray(['order.received'], new Utf8()),
    body: vectorFromArray(['received'], new Utf8()),
  })
}

export function logDetailTable(): Table {
  const entries = new Struct([
    new Field('key', new Utf8(), false),
    new Field('value', new Int64(), true),
  ])
  const attrs = new Map_(new Field('entries', entries, false))
  const resource = new Struct([
    new Field('attributes', attrs),
    new Field('droppedAttributesCount', new Int32()),
    new Field('schemaUrl', new Utf8()),
  ])
  const scope = new Struct([
    ...resource.children,
    new Field('name', new Utf8()),
    new Field('version', new Utf8()),
  ])
  return new Table({
    logRef: vectorFromArray(['log-1'], new Utf8()),
    timestamp: vectorFromArray([100n], new Uint64()),
    observedTimestamp: vectorFromArray([101n], new Uint64()),
    traceID: vectorFromArray([null], new Utf8()),
    spanID: vectorFromArray([null], new Utf8()),
    severityText: vectorFromArray(['INFO'], new Utf8()),
    severityNumber: vectorFromArray([9], new Int32()),
    eventName: vectorFromArray([''], new Utf8()),
    flags: vectorFromArray([0], new Int32()),
    droppedAttributesCount: vectorFromArray([0], new Int32()),
    body: vectorFromArray([9223372036854775807n], new Int64()),
    attributes: vectorFromArray([new Map()], attrs),
    resource: vectorFromArray(
      [{ attributes: new Map(), droppedAttributesCount: 0, schemaUrl: '' }],
      resource
    ),
    scope: vectorFromArray(
      [
        {
          attributes: new Map(),
          droppedAttributesCount: 0,
          schemaUrl: '',
          name: '',
          version: '',
        },
      ],
      scope
    ),
  })
}
