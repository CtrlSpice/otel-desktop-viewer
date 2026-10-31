import type {
  AttributeValue,
  AggregateBucket,
  DataPoint,
  LogData,
  MetricSeriesViewData,
  MetricAggregateViewData,
  MetricViewData,
  ResourceData,
  ScopeData,
  SpanData,
  SpanNode,
  TraceData,
} from '../types/api-types'

/** Readable test inputs; the fixture writer encodes these directly into Arrow. */
export type Fixture<T> = T extends bigint
  ? bigint | string | number
  : T extends object
    ? { [K in keyof T]: Fixture<T[K]> }
    : T

export type AttributeFixture = Fixture<AttributeValue>
export type LogFixture = Fixture<LogData>
export type PointFixture = Fixture<DataPoint>
export type AggregateFixture = Fixture<AggregateBucket>
export type SeriesFixture = Fixture<MetricSeriesViewData>

/** Compact authoring form: share contexts and specify exact relative timings. */
export type TraceFixture = Fixture<
  Omit<TraceData, 'spans'> & {
    traceStart: bigint
    resources: Record<string, ResourceData>
    scopes: Record<string, ScopeData>
    spans: (Omit<SpanNode, 'spanData'> & {
      spanData: Omit<
        SpanData,
        'traceID' | 'startTime' | 'endTime' | 'resource' | 'scope'
      > & { start: bigint; dur: bigint; r: number; s: number }
    })[]
  }
>

/** One test scenario can supply both the view and its separate aggregate read. */
export type MetricFixture = Fixture<MetricViewData & MetricAggregateViewData>
