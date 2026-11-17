import type {
  WaterfallSpanNode as SpanNode,
  TraceLogSummary,
} from '@/types/api-types'
import type { TreeConnectorMeta } from '@/components/shared/TreeGutter.svelte'
import { getServiceName } from '@/utils/resource'
import {
  spanNameColor,
  traceServiceColor,
} from '@/utils/colours/trace-service-color'
import { recordsForSpan } from './timeline-markers'
import { computeTreeMeta, isErrorSpan } from './waterfall-tree'

export type TraceBounds = {
  start: bigint
  end: bigint
  duration: bigint
}

export type WaterfallRowData = {
  spanNode: SpanNode
  /** CSS-ready colour string for the bar / gutter / event dot.
   *  Error spans pass `--color-error` via CSS var to preserve semantic
   *  theming; non-error spans get a concrete approved palette colour. */
  color: string
  /** Whether this row is an error span. Consumers branch on this for the
   *  matched/error tinting (which uses semantic vars, not the palette). */
  isError: boolean
  offsetPercent: number
  widthPercent: number
  tree: TreeConnectorMeta
  records: import('./timeline-markers').TimelineRecord[]
}

export function getTraceBounds(spans: SpanNode[]): TraceBounds {
  if (spans.length === 0) {
    return { start: 0n, end: 0n, duration: 0n }
  }
  const seed = {
    start: spans[0].spanData.startTime,
    end: spans[0].spanData.endTime,
  }
  const { start, end } = spans.reduce((acc, node) => {
    const st = node.spanData.startTime
    const en = node.spanData.endTime
    return {
      start: st < acc.start ? st : acc.start,
      end: en > acc.end ? en : acc.end,
    }
  }, seed)
  return { start, end, duration: end - start }
}

function getOffsetPercent(
  traceStart: bigint,
  traceDuration: bigint,
  point: bigint
): number {
  if (traceDuration <= 0n) return 0
  return Number(((point - traceStart) * 10000n) / traceDuration) / 100
}

function getWidthPercent(traceDuration: bigint, spanDuration: bigint): number {
  if (traceDuration <= 0n) return 0
  return Math.max(0.3, Number((spanDuration * 10000n) / traceDuration) / 100)
}

/** Use span names for a single service, otherwise Home service colours. */
export function buildWaterfallRows(
  spans: SpanNode[],
  bounds: TraceBounds,
  theme: string,
  logsBySpanID: ReadonlyMap<string, TraceLogSummary[]> = new Map()
): WaterfallRowData[] {
  const treeMeta = computeTreeMeta(spans)
  const services = spans.map(
    node => getServiceName(node.spanData.resource) ?? ''
  )
  const singleService = new Set(services).size <= 1

  return spans.map((node, i) => {
    const isError = isErrorSpan(node.spanData)
    const color = isError
      ? 'var(--color-error)'
      : singleService
        ? spanNameColor(node.spanData.name, theme)
        : traceServiceColor(services[i]!, theme)
    return {
      spanNode: node,
      color,
      isError,
      offsetPercent: getOffsetPercent(
        bounds.start,
        bounds.duration,
        node.spanData.startTime
      ),
      widthPercent: getWidthPercent(
        bounds.duration,
        node.spanData.endTime - node.spanData.startTime
      ),
      tree: treeMeta[i]!,
      records: recordsForSpan(
        node,
        logsBySpanID.get(node.spanData.spanID) ?? []
      ),
    }
  })
}
