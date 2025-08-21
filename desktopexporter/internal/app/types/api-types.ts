import { PreciseTimestamp } from './precise-timestamp';

export type RootSpan = {
  serviceName: string;
  name: string;
  startTime: PreciseTimestamp;
  endTime: PreciseTimestamp;
};

export type TraceSummary = {
  traceID: string;
  rootSpan?: RootSpan;
  spanCount: number;
};

export type TraceData = {
  traceID: string;
  spans: SpanNode[];
};

export type SpanNode = {
  spanData: SpanData;
  depth: number;
};

export type SpanData = {
  traceID: string;
  traceState: string;
  spanID: string;
  parentSpanID: string;

  name: string;
  kind: string;
  startTime: PreciseTimestamp;
  endTime: PreciseTimestamp;

  attributes: Attributes;
  events: EventData[];
  links: LinkData[];
  resource: ResourceData;
  scope: ScopeData;

  droppedAttributesCount: number;
  droppedEventsCount: number;
  droppedLinksCount: number;

  statusCode: string;
  statusMessage: string;
};

export type Attributes = Record<string, string | number | boolean | string[] | number[] | boolean[]>;

export type ResourceData = {
  attributes: Attributes;
  droppedAttributesCount: number;
};

export type ScopeData = {
  name: string;
  version: string;
  attributes: Attributes;
  droppedAttributesCount: number;
};

export type EventData = {
  name: string;
  timestamp: PreciseTimestamp;
  attributes: Attributes;
  droppedAttributesCount: number;
};

export type LinkData = {
  traceID: string;
  spanID: string;
  traceState: string;
  attributes: Attributes;
  droppedAttributesCount: number;
};

export type LogData = {
  timestamp: PreciseTimestamp;
  observedTimestamp: PreciseTimestamp;
  traceID: string;
  spanID: string;
  severityText: string;
  severityNumber: number;
  body: string | object;
  resource: ResourceData;
  scope: ScopeData;
  attributes: Attributes;
  droppedAttributesCount: number;
  flags: number;
  eventName: string;
};





// Metrics types
export type MetricType = 'Empty' | 'Gauge' | 'Sum' | 'Histogram' | 'ExponentialHistogram';

export type Exemplar = {
  timestamp: PreciseTimestamp;
  value: number;
  filteredAttributes: Attributes;
  traceID: string;
  spanID: string;
};

export type MetricDataPoint = GaugeDataPoint | SumDataPoint | HistogramDataPoint | ExponentialHistogramDataPoint;

export type GaugeDataPoint = {
  timestamp: PreciseTimestamp;
  startTime: PreciseTimestamp;
  attributes: Attributes;
  flags: number;
  valueType: string;
  value: number;
  exemplars?: Exemplar[];
};

export type SumDataPoint = {
  timestamp: PreciseTimestamp;
  startTime: PreciseTimestamp;
  attributes: Attributes;
  flags: number;
  valueType: string;
  value: number;
  exemplars?: Exemplar[];
  isMonotonic: boolean;
  aggregationTemporality: string;
};

export type HistogramDataPoint = {
  timestamp: PreciseTimestamp;
  startTime: PreciseTimestamp;
  attributes: Attributes;
  flags: number;
  count: number;
  sum: number;
  min: number;
  max: number;
  bounds: number[];
  counts: number[];
  exemplars?: Exemplar[];
  aggregationTemporality: string;
};

export type ExponentialHistogramDataPoint = {
  timestamp: PreciseTimestamp;
  startTime: PreciseTimestamp;
  attributes: Attributes;
  flags: number;
  count: number;
  sum: number;
  min: number;
  max: number;
  scale: number;
  zeroCount: number;
  positive: {
    offset: number;
    bucketCounts: number[];
  };
  negative: {
    offset: number;
    bucketCounts: number[];
  };
  exemplars?: Exemplar[];
  aggregationTemporality: string;
};

export type DataPoints = {
  type: MetricType;
  points: MetricDataPoint[];
};

export type MetricData = {
  name: string;
  description: string;
  unit: string;
  dataPoints: DataPoints;
  resource: ResourceData;
  scope: ScopeData;
  received: PreciseTimestamp;
};


