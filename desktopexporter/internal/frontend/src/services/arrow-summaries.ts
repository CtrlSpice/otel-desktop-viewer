import type {
  LogSummary,
  TraceLogSummary,
  TraceSummary,
  MetricSummary,
  MetricType,
  LogTableRow,
  LogColumn,
  LogData,
  AttributeValue,
} from '@/types/api-types'
import { ArrowFrame } from './arrow-data'
import { attributeValue, attributes, rowViews } from './arrow-values'

function text(frame: ArrowFrame, row: number, name: string): string {
  const cell = frame.cell(name, row)
  return cell.isNull ? '' : cell.text()
}

function nullableText(
  frame: ArrowFrame,
  row: number,
  name: string
): string | null {
  const cell = frame.cell(name, row)
  return cell.isNull ? null : cell.text()
}

function attributeKind(frame: ArrowFrame, row: number, name: string): AttributeValue['kind'] {
  const kind = text(frame, row, name)
  switch (kind) {
    case 'empty':
    case 'string':
    case 'int64':
    case 'double':
    case 'bool':
    case 'bytes':
    case 'map':
    case 'array':
      return kind
    default:
      throw new Error(`Unsupported attribute type: ${kind}`)
  }
}

export function traceSummary(frame: ArrowFrame, row: number): TraceSummary {
  return {
    get traceID() {
      return frame.cell('traceID', row).text()
    },
    get hasRootSpan() {
      return frame.cell('hasRootSpan', row).boolean()
    },
    get rootSpan() {
      const cell = frame.cell('rootSpan', row)
      if (cell.isNull) return undefined
      const root = cell.struct()
      return {
        get serviceName() {
          return text(root, 0, 'serviceName')
        },
        get name() {
          return text(root, 0, 'name')
        },
      }
    },
    get startTime() {
      return frame.cell('startTime', row).integer()
    },
    get durationNs() {
      const end = frame.cell('endTime', row)
      return end.isNull
        ? null
        : end.integer() - frame.cell('startTime', row).integer()
    },
    get spanCount() {
      return frame.cell('spanCount', row).number()
    },
    get errorCount() {
      return frame.cell('errorCount', row).number()
    },
    get serviceBreakdown() {
      const cell = frame.cell('serviceBreakdown', row)
      if (cell.isNull) return []
      return rowViews(new ArrowFrame(cell.list()), (parts, i) => ({
        get key() {
          return parts.cell('key', i).text()
        },
        get label() {
          return parts.cell('label', i).text()
        },
        get count() {
          return parts.cell('count', i).number()
        },
        get other() {
          return parts.cell('other', i).boolean()
        },
      }))
    },
    get matchedSpans() {
      if (!frame.has('matchedSpans')) return undefined
      const cell = frame.cell('matchedSpans', row)
      if (cell.isNull) return undefined
      return rowViews(new ArrowFrame(cell.list()), (matches, i) => ({
        get traceID() {
          return matches.cell('traceID', i).text()
        },
        get spanID() {
          return matches.cell('spanID', i).text()
        },
      }))
    },
  }
}

export function logSummary(frame: ArrowFrame, row: number): LogSummary {
  return {
    get logRef() {
      return frame.cell('logRef', row).text()
    },
    get timestamp() {
      return frame.cell('timestamp', row).integer()
    },
    get severityText() {
      return text(frame, row, 'severityText')
    },
    get severityNumber() {
      return frame.cell('severityNumber', row).number()
    },
    get eventName() {
      return text(frame, row, 'eventName')
    },
    get serviceName() {
      return text(frame, row, 'serviceName')
    },
    get bodyPreview() {
      return text(frame, row, 'bodyPreview')
    },
    get bodyType() {
      return attributeKind(frame, row, 'bodyType')
    },
  }
}

export function traceLogSummary(
  frame: ArrowFrame,
  row: number
): TraceLogSummary {
  // SAFETY: Copying every LogSummary property descriptor supplies all summary
  // fields; the target supplies the required eventName and nullable spanID.
  return Object.defineProperties(
    {
      get eventName() {
        return text(frame, row, 'eventName')
      },
      get spanID() {
        return nullableText(frame, row, 'spanID')
      },
    },
    Object.getOwnPropertyDescriptors(logSummary(frame, row))
  ) as TraceLogSummary
}

export function logTableRow(
  frame: ArrowFrame,
  row: number,
  columns: readonly LogColumn[]
): LogTableRow {
  // SAFETY: The descriptors supply every LogSummary field without reading it;
  // the target adds the typed optional columnValues getter.
  return Object.defineProperties(
    {
      get columnValues() {
        if (!frame.has('columnValues')) return undefined
        const values = new ArrowFrame(frame.cell('columnValues', row).list())
        return columns.map((column, i) => ({
          ...column,
          get present() {
            return values.cell('present', i).boolean()
          },
          get valueType() {
            return attributeKind(values, i, 'valueType')
          },
          get preview() {
            return text(values, i, 'preview')
          },
        }))
      },
    },
    Object.getOwnPropertyDescriptors(logSummary(frame, row))
  ) as LogTableRow
}

export function logData(frame: ArrowFrame, row: number): LogData {
  const resource = frame.cell('resource', row).struct()
  const scope = frame.cell('scope', row).struct()
  return {
    get logRef() {
      return frame.cell('logRef', row).text()
    },
    get timestamp() {
      return frame.cell('timestamp', row).integer()
    },
    get observedTimestamp() {
      return frame.cell('observedTimestamp', row).integer()
    },
    get traceID() {
      return nullableText(frame, row, 'traceID')
    },
    get spanID() {
      return nullableText(frame, row, 'spanID')
    },
    get severityText() {
      return text(frame, row, 'severityText')
    },
    get severityNumber() {
      return frame.cell('severityNumber', row).number()
    },
    get eventName() {
      return text(frame, row, 'eventName')
    },
    get flags() {
      return frame.cell('flags', row).number()
    },
    get body() {
      return attributeValue(frame.cell('body', row))
    },
    get attributes() {
      return attributes(frame.cell('attributes', row))
    },
    get droppedAttributesCount() {
      return frame.cell('droppedAttributesCount', row).number()
    },
    resource: {
      get attributes() {
        return attributes(resource.cell('attributes', 0))
      },
      get droppedAttributesCount() {
        return resource.cell('droppedAttributesCount', 0).number()
      },
    },
    scope: {
      get name() {
        return text(scope, 0, 'name')
      },
      get version() {
        return text(scope, 0, 'version')
      },
      get attributes() {
        return attributes(scope.cell('attributes', 0))
      },
      get droppedAttributesCount() {
        return scope.cell('droppedAttributesCount', 0).number()
      },
    },
    get resourceSchemaURL() {
      return text(resource, 0, 'schemaUrl')
    },
    get scopeSchemaURL() {
      return text(scope, 0, 'schemaUrl')
    },
  }
}

function metricType(frame: ArrowFrame, row: number): MetricType {
  const type = frame.cell('metricType', row).text()
  switch (type) {
    case 'Empty':
    case 'Gauge':
    case 'Sum':
    case 'Histogram':
    case 'ExponentialHistogram':
      return type
    default:
      throw new Error(`Unsupported metric type: ${type}`)
  }
}

export function metricSummary(frame: ArrowFrame, row: number): MetricSummary {
  return {
    get metricRef() {
      return frame.cell('metricRef', row).text()
    },
    get name() {
      return text(frame, row, 'name')
    },
    get unit() {
      return text(frame, row, 'unit')
    },
    get metricType() {
      return metricType(frame, row)
    },
    get aggregationTemporalityCode() {
      const cell = frame.cell('aggregationTemporalityCode', row)
      return cell.isNull ? null : cell.number()
    },
    get aggregationTemporality() {
      const cell = frame.cell('aggregationTemporalityCode', row)
      if (cell.isNull) return null
      const code = cell.number()
      return ['Unspecified', 'Delta', 'Cumulative'][code] ?? `Unknown (${code})`
    },
    get isMonotonic() {
      const cell = frame.cell('isMonotonic', row)
      return cell.isNull ? null : cell.boolean()
    },
    get serviceName() {
      return text(frame, row, 'serviceName')
    },
    get seriesCount() {
      return frame.cell('seriesCount', row).number()
    },
    get seriesCardinality() {
      return frame.cell('seriesCardinality', row).number()
    },
    get dataPointCount() {
      return frame.cell('dataPointCount', row).number()
    },
    get lastValue() {
      const cell = frame.cell('lastValue', row)
      return cell.isNull ? null : cell.number()
    },
    get lastSeen() {
      const cell = frame.cell('lastSeen', row)
      return cell.isNull ? null : cell.integer()
    },
  }
}
