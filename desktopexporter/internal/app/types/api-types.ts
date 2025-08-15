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
  spans: SpanData[];
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



// Helper functions to deserialize timestamps
export function traceSummaryFromJSON(json: any): TraceSummary {
  return {
    ...json,
    rootSpan: json.rootSpan ? {
      ...json.rootSpan,
      startTime: PreciseTimestamp.fromJSON(json.rootSpan.startTime),
      endTime: PreciseTimestamp.fromJSON(json.rootSpan.endTime)
    } : undefined
  };
}

export function traceSummariesFromJSON(json: any): TraceSummary[] {
  return json.map(traceSummaryFromJSON);
}

export function traceDataFromJSON(json: any): TraceData {
  return {
    ...json,
    spans: json.spans.map((span: any) => ({
      ...span,
      startTime: PreciseTimestamp.fromJSON(span.startTime),
      endTime: PreciseTimestamp.fromJSON(span.endTime),
      events: span.events?.map((event: any) => ({
        ...event,
        timestamp: PreciseTimestamp.fromJSON(event.timestamp)
      }))
    }))
  };
}

export function logsFromJSON(json: any): LogData[] {
  return json.map((log: any) => ({
    ...log,
    timestamp: PreciseTimestamp.fromJSON(log.timestamp),
    observedTimestamp: PreciseTimestamp.fromJSON(log.observedTimestamp)
  }));
}

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

// Helper functions to deserialize metrics timestamps
export function exemplarFromJSON(json: any): Exemplar {
  return {
    ...json,
    timestamp: PreciseTimestamp.fromJSON(json.timestamp),
  };
}

export function gaugeDataPointFromJSON(json: any): GaugeDataPoint {
  return {
    ...json,
    timestamp: PreciseTimestamp.fromJSON(json.timestamp),
    startTime: PreciseTimestamp.fromJSON(json.startTimeUnixNano),
    exemplars: json.exemplars?.map(exemplarFromJSON),
  };
}

export function sumDataPointFromJSON(json: any): SumDataPoint {
  return {
    ...json,
    timestamp: PreciseTimestamp.fromJSON(json.timestamp),
    startTime: PreciseTimestamp.fromJSON(json.startTimeUnixNano),
    exemplars: json.exemplars?.map(exemplarFromJSON),
  };
}

export function histogramDataPointFromJSON(json: any): HistogramDataPoint {
  return {
    ...json,
    timestamp: PreciseTimestamp.fromJSON(json.timestamp),
    startTime: PreciseTimestamp.fromJSON(json.startTimeUnixNano),
    exemplars: json.exemplars?.map(exemplarFromJSON),
  };
}

export function exponentialHistogramDataPointFromJSON(json: any): ExponentialHistogramDataPoint {
  return {
    ...json,
    timestamp: PreciseTimestamp.fromJSON(json.timestamp),
    startTime: PreciseTimestamp.fromJSON(json.startTimeUnixNano),
    exemplars: json.exemplars?.map(exemplarFromJSON),
  };
}

export function dataPointsFromJSON(json: any): DataPoints {
  const points = json.points.map((point: any) => {
    switch (json.type) {
      case 'Gauge':
        return gaugeDataPointFromJSON(point);
      case 'Sum':
        return sumDataPointFromJSON(point);
      case 'Histogram':
        return histogramDataPointFromJSON(point);
      case 'ExponentialHistogram':
        return exponentialHistogramDataPointFromJSON(point);
      default:
        return point; // For Empty type or unknown types
    }
  });

  return {
    type: json.type,
    points,
  };
}

export function metricDataFromJSON(json: any): MetricData {
  return {
    ...json,
    dataPoints: dataPointsFromJSON(json.dataPoints),
    received: PreciseTimestamp.fromJSON(json.received),
  };
}

export function metricsFromJSON(json: any): MetricData[] {
  return json.map(metricDataFromJSON);
}
