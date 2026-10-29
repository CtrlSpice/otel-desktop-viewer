import type {
  TraceData,
  SpanData,
  SpanNode,
  EventData,
  LinkData,
  ResourceData,
  ScopeData,
} from '@/types/api-types'
import { ArrowFrame } from './arrow-data'
import { attributes, rowViews } from './arrow-values'

function text(frame: ArrowFrame, row: number, name: string): string {
  const cell = frame.cell(name, row)
  return cell.isNull ? '' : cell.text()
}

export function resourceData(frame: ArrowFrame, row: number): ResourceData {
  return {
    get attributes() {
      return attributes(frame.cell('attributes', row))
    },
    get droppedAttributesCount() {
      return frame.cell('droppedAttributesCount', row).number()
    },
  }
}

export function scopeData(frame: ArrowFrame, row: number): ScopeData {
  return {
    get name() {
      return text(frame, row, 'name')
    },
    get version() {
      return text(frame, row, 'version')
    },
    get attributes() {
      return attributes(frame.cell('attributes', row))
    },
    get droppedAttributesCount() {
      return frame.cell('droppedAttributesCount', row).number()
    },
  }
}

function eventData(frame: ArrowFrame, row: number): EventData {
  return {
    get name() {
      return text(frame, row, 'name')
    },
    get timestamp() {
      return frame.cell('timestamp', row).integer()
    },
    get attributes() {
      return attributes(frame.cell('attributes', row))
    },
    get droppedAttributesCount() {
      return frame.cell('droppedAttributesCount', row).number()
    },
  }
}

function linkData(frame: ArrowFrame, row: number): LinkData {
  return {
    get traceID() {
      const cell = frame.cell('traceID', row)
      return cell.isNull ? null : cell.text()
    },
    get spanID() {
      const cell = frame.cell('spanID', row)
      return cell.isNull ? null : cell.text()
    },
    get traceState() {
      return text(frame, row, 'traceState')
    },
    get flags() {
      return frame.cell('flags', row).number()
    },
    get attributes() {
      return attributes(frame.cell('attributes', row))
    },
    get droppedAttributesCount() {
      return frame.cell('droppedAttributesCount', row).number()
    },
    get summary() {
      const target = frame.cell('summary', row).struct().cell('target', 0)
      if (target.isNull) return { target: null }
      const data = target.struct()
      return {
        target: {
          get name() {
            return text(data, 0, 'name')
          },
          get serviceName() {
            return text(data, 0, 'serviceName')
          },
        },
      }
    },
  }
}

/** Native timestamps stay absolute; resource/scope references share lazy views. */
export function traceData(frame: ArrowFrame): TraceData {
  const traceID = frame.cell('traceID', 0).text()
  const resources = rowViews(
    new ArrowFrame(frame.cell('resources', 0).list()),
    resourceData
  )
  const scopes = rowViews(
    new ArrowFrame(frame.cell('scopes', 0).list()),
    scopeData
  )
  const spans = rowViews(
    new ArrowFrame(frame.cell('spans', 0).list()),
    (nodes, row): SpanNode => {
      const data = nodes.cell('spanData', row).struct()
      const spanData: SpanData = {
        traceID,
        get traceState() {
          return text(data, 0, 'traceState')
        },
        get spanID() {
          return data.cell('spanID', 0).text()
        },
        get parentSpanID() {
          const cell = data.cell('parentSpanID', 0)
          return cell.isNull ? null : cell.text()
        },
        get flags() {
          return data.cell('flags', 0).number()
        },
        get name() {
          return text(data, 0, 'name')
        },
        get kindCode() {
          return data.cell('kindCode', 0).number()
        },
        get kind() {
          const code = data.cell('kindCode', 0).number()
          return (
            [
              'Unspecified',
              'Internal',
              'Server',
              'Client',
              'Producer',
              'Consumer',
            ][code] ?? `Unknown (${code})`
          )
        },
        get startTime() {
          return data.cell('startTime', 0).integer()
        },
        get endTime() {
          return data.cell('endTime', 0).integer()
        },
        get attributes() {
          return attributes(data.cell('attributes', 0))
        },
        get events() {
          return rowViews(
            new ArrowFrame(data.cell('events', 0).list()),
            eventData
          )
        },
        get links() {
          return rowViews(
            new ArrowFrame(data.cell('links', 0).list()),
            linkData
          )
        },
        get resource() {
          return resources[data.cell('r', 0).number() - 1]!
        },
        get scope() {
          return scopes[data.cell('s', 0).number() - 1]!
        },
        get droppedAttributesCount() {
          return data.cell('droppedAttributesCount', 0).number()
        },
        get droppedEventsCount() {
          return data.cell('droppedEventsCount', 0).number()
        },
        get droppedLinksCount() {
          return data.cell('droppedLinksCount', 0).number()
        },
        get statusCodeValue() {
          return data.cell('statusCodeValue', 0).number()
        },
        get statusCode() {
          const code = data.cell('statusCodeValue', 0).number()
          return ['Unset', 'Ok', 'Error'][code] ?? `Unknown (${code})`
        },
        get statusMessage() {
          return text(data, 0, 'statusMessage')
        },
      }
      const node: SpanNode = {
        spanData,
        get depth() {
          return nodes.cell('depth', row).number()
        },
        get matched() {
          return nodes.cell('matched', row).boolean()
        },
      }
      if (nodes.has('salvaged') && nodes.cell('salvaged', row).boolean()) {
        node.salvaged = true
        node.cyclePoint = nodes.cell('cyclePoint', row).boolean()
      }
      return node
    }
  )
  return {
    traceID,
    get unplacedSpanCount() {
      return frame.cell('unplacedSpanCount', 0).number()
    },
    spans,
  }
}
