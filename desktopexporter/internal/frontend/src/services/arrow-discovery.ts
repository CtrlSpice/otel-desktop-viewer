import { ArrowFrame, type ArrowCell } from './arrow-data'
import { cachedColumnView as columnView, number, text } from './arrow-record'
import { rowViews } from './arrow-values'
import type {
  AttributeKey,
  DiscoveryPage,
  DiscoveryTotals,
  ServiceConnection,
  ServiceConnectionSpan,
  ServiceSummary,
} from './discovery-types'

const timestamp = (cell: ArrowCell): string => cell.integer().toString()

export function serviceSummary(frame: ArrowFrame, row: number): ServiceSummary {
  return columnView<ServiceSummary>(frame, row, {
    serviceColorKey: cell => (cell.isNull ? undefined : text(cell)),
    labelsComplete: cell => (cell.isNull ? undefined : cell.boolean()),
    serviceName: text,
    serviceNamespace: text,
    serviceIdentity: text,
    serviceNameKind: text,
    serviceNamespaceKind: text,
    traceCount: number,
    spanCount: number,
    errorSpanCount: number,
    logCount: number,
    errorLogCount: number,
    metricCount: number,
    dataPointCount: number,
    exemplarCount: number,
    lastSeen: cell => (cell.isNull ? null : timestamp(cell)),
    logSeverityCounts: cell =>
      columnView<ServiceSummary['logSeverityCounts']>(cell.struct(), 0, {
        trace: number,
        debug: number,
        info: number,
        warn: number,
        error: number,
        fatal: number,
        other: number,
      }),
  })
}

export function serviceConnection(
  frame: ArrowFrame,
  row: number
): ServiceConnection {
  return columnView<ServiceConnection>(frame, row, {
    source: cell => serviceSummary(cell.struct(), 0),
    target: cell => serviceSummary(cell.struct(), 0),
    spanCount: number,
    traceCount: number,
    errorSpanCount: number,
  })
}

export function serviceConnectionSpan(
  frame: ArrowFrame,
  row: number
): ServiceConnectionSpan {
  return columnView<ServiceConnectionSpan>(frame, row, {
    traceID: text,
    spanID: text,
    name: text,
    startTime: timestamp,
    endTime: timestamp,
    statusCode: number,
  })
}

function foundOn(cell: ArrowCell): AttributeKey['foundOn'] {
  return rowViews(new ArrowFrame(cell.list()), (frame, row) => {
    const signal = frame.cell('signal', row).text()
    const ownerType = frame.cell('ownerType', row).text()
    if (signal !== 'traces' && signal !== 'logs' && signal !== 'metrics')
      throw new Error(`Unknown discovery signal: ${signal}`)
    if (
      ownerType !== 'resource' &&
      ownerType !== 'scope' &&
      ownerType !== 'span' &&
      ownerType !== 'event' &&
      ownerType !== 'link' &&
      ownerType !== 'log' &&
      ownerType !== 'datapoint' &&
      ownerType !== 'exemplar' &&
      ownerType !== 'metadata'
    )
      throw new Error(`Unknown discovery owner: ${ownerType}`)
    return { signal, ownerType }
  })
}

export function attributeKey(frame: ArrowFrame, row: number): AttributeKey {
  return columnView<AttributeKey>(frame, row, {
    key: text,
    kind: text,
    foundOn,
  })
}

export function discoveryPage<T>(
  frame: ArrowFrame,
  view: (frame: ArrowFrame, row: number) => T
): DiscoveryPage<T> {
  return {
    items: rowViews(new ArrowFrame(frame.cell('items', 0).list()), view),
    truncated: frame.cell('truncated', 0).boolean(),
    totals:
      frame.has('totals') && !frame.cell('totals', 0).isNull
        ? columnView<DiscoveryTotals>(frame.cell('totals', 0).struct(), 0, {
            home: number,
            traces: number,
            metrics: number,
            logs: number,
          })
        : undefined,
  }
}
