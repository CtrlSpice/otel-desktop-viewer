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

export type TraceSummaries = {
  traceSummaries: TraceSummary[];
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

export type Logs = {
  logs: LogData[];
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

export function traceSummariesFromJSON(json: any): TraceSummaries {
  return {
    traceSummaries: json.traceSummaries.map(traceSummaryFromJSON)
  };
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

export function logsFromJSON(json: any): Logs {
  return {
    logs: json.logs.map((log: any) => ({
      ...log,
      timestamp: PreciseTimestamp.fromJSON(log.timestamp),
      observedTimestamp: PreciseTimestamp.fromJSON(log.observedTimestamp)
    }))
  };
}
