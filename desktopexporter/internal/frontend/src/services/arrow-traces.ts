import type {
  TraceData,
  SpanData,
  SpanNode,
  EventData,
  LinkData,
  ResourceData,
  ScopeData,
  TraceWaterfallData,
  TraceColumnPreview,
  WaterfallSpanData,
  WaterfallSpanNode,
} from '@/types/api-types'
import { ArrowFrame } from './arrow-data'
import { attributes, rowViews } from './arrow-values'
import { cachedView } from './arrow-record'

function text(frame: ArrowFrame, row: number, name: string): string {
  const cell = frame.cell(name, row)
  return cell.isNull ? '' : cell.text()
}

function columnSource(value: string): TraceColumnPreview['source'] {
  if (value === 'span' || value === 'resource' || value === 'scope')
    return value
  throw new Error(`Unsupported trace column source: ${value}`)
}

function previewValueType(
  value: string
): TraceColumnPreview['values'][number]['valueType'] {
  switch (value) {
    case 'empty':
    case 'string':
    case 'bytes':
    case 'bool':
    case 'int64':
    case 'double':
    case 'map':
    case 'array':
      return value
    default:
      throw new Error(`Unsupported trace preview type: ${value}`)
  }
}

export function resourceData(frame: ArrowFrame, row: number): ResourceData {
  return cachedView({
    get attributes() {
      return attributes(frame.cell('attributes', row))
    },
    get droppedAttributesCount() {
      return frame.cell('droppedAttributesCount', row).number()
    },
  })
}

export function scopeData(frame: ArrowFrame, row: number): ScopeData {
  return cachedView({
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
  })
}

function eventData(frame: ArrowFrame, row: number): EventData {
  return cachedView({
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
  })
}

function linkData(frame: ArrowFrame, row: number): LinkData {
  return cachedView({
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
        target: cachedView({
          get name() {
            return text(data, 0, 'name')
          },
          get serviceName() {
            return text(data, 0, 'serviceName')
          },
        }),
      }
    },
  })
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
      const spanData: SpanData = cachedView({
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
        get resourceSchemaURL() {
          const owners = new ArrowFrame(frame.cell('resources', 0).list())
          return owners.has('schemaUrl')
            ? text(owners, data.cell('r', 0).number() - 1, 'schemaUrl')
            : ''
        },
        get scopeSchemaURL() {
          const owners = new ArrowFrame(frame.cell('scopes', 0).list())
          return owners.has('schemaUrl')
            ? text(owners, data.cell('s', 0).number() - 1, 'schemaUrl')
            : ''
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
      })
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

/** Tree-only views retain native nested buffers; no full span objects are fabricated. */
export function traceWaterfallData(frame: ArrowFrame): TraceWaterfallData {
  const traceID = frame.cell('traceID', 0).text()
  const resources = new ArrowFrame(frame.cell('resources', 0).list())
  const scopes = new ArrowFrame(frame.cell('scopes', 0).list())
  const resourceViews = rowViews(resources, (f, i) => ({
    serviceName: text(f, i, 'serviceName'),
    droppedAttributesCount: f.cell('droppedAttributesCount', i).number(),
  }))
  const scopeViews = rowViews(scopes, (f, i) => ({
    name: text(f, i, 'name'),
    version: text(f, i, 'version'),
    droppedAttributesCount: f.cell('droppedAttributesCount', i).number(),
  }))
  const nodes = new ArrowFrame(frame.cell('spans', 0).list())
  return {
    traceID,
    unplacedSpanCount: frame.cell('unplacedSpanCount', 0).number(),
    availableColumns: rowViews(
      new ArrowFrame(frame.cell('availableColumns', 0).list()),
      (f, i) => ({
        source: columnSource(text(f, i, 'source')),
        key: text(f, i, 'key'),
        count: f.cell('count', i).number(),
      })
    ),
    spans: rowViews(nodes, (f, i) => {
      const data = f.cell('spanData', i).struct()
      const r = data.cell('r', 0).number() - 1
      const s = data.cell('s', 0).number() - 1
      const kindCode = data.cell('kindCode', 0).number()
      const statusCodeValue = data.cell('statusCodeValue', 0).number()
      const spanData: WaterfallSpanData = {
        traceID,
        spanID: text(data, 0, 'spanID'),
        parentSpanID: data.cell('parentSpanID', 0).isNull
          ? null
          : text(data, 0, 'parentSpanID'),
        name: text(data, 0, 'name'),
        get traceState() {
          return text(data, 0, 'traceState')
        },
        flags: data.cell('flags', 0).number(),
        kindCode,
        kind:
          [
            'Unspecified',
            'Internal',
            'Server',
            'Client',
            'Producer',
            'Consumer',
          ][kindCode] ?? `Unknown (${kindCode})`,
        startTime: data.cell('startTime', 0).integer(),
        endTime: data.cell('endTime', 0).integer(),
        statusCodeValue,
        statusCode:
          ['Unset', 'Ok', 'Error'][statusCodeValue] ??
          `Unknown (${statusCodeValue})`,
        get statusMessage() {
          return text(data, 0, 'statusMessage')
        },
        droppedAttributesCount: data.cell('droppedAttributesCount', 0).number(),
        droppedEventsCount: data.cell('droppedEventsCount', 0).number(),
        droppedLinksCount: data.cell('droppedLinksCount', 0).number(),
        events: rowViews(
          new ArrowFrame(data.cell('events', 0).list()),
          (events, row) =>
            cachedView({
              name: text(events, row, 'name'),
              timestamp: events.cell('timestamp', row).integer(),
              droppedAttributesCount: events
                .cell('droppedAttributesCount', row)
                .number(),
              get summaryAttributes() {
                return attributes(events.cell('summaryAttributes', row))
              },
            })
        ),
        resource: resourceViews[r]!,
        scope: scopeViews[s]!,
        columnValues: data.has('columnValues')
          ? rowViews(
              new ArrowFrame(data.cell('columnValues', 0).list()),
              (c, row) => ({
                spanID: text(data, 0, 'spanID'),
                source: columnSource(text(c, row, 'source')),
                key: text(c, row, 'key'),
                values: rowViews(
                  new ArrowFrame(c.cell('values', row).list()),
                  (v, j) => ({
                    valueType: previewValueType(text(v, j, 'valueType')),
                    preview: text(v, j, 'preview'),
                  })
                ),
              })
            )
          : [],
      }
      const node: WaterfallSpanNode = {
        spanData,
        depth: f.cell('depth', i).number(),
        matched: f.cell('matched', i).boolean(),
      }
      if (f.has('salvaged') && f.cell('salvaged', i).boolean()) {
        node.salvaged = true
        node.cyclePoint = f.cell('cyclePoint', i).boolean()
      }
      return node
    }),
  }
}

export function traceColumnPreviews(frame: ArrowFrame): TraceColumnPreview[] {
  return rowViews(frame, (f, i) => ({
    spanID: text(f, i, 'spanID'),
    source: columnSource(text(f, i, 'source')),
    key: text(f, i, 'key'),
    values: rowViews(new ArrowFrame(f.cell('values', i).list()), (v, j) => ({
      valueType: previewValueType(text(v, j, 'valueType')),
      preview: text(v, j, 'preview'),
    })),
  }))
}
