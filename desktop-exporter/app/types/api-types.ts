export type TraceSummary = {
  hasRootSpan: boolean;
  rootServiceName: string;
  rootName: string;
  rootStartTime: string;
  rootEndTime: string;
  spanCount: number;
  traceID: string;
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
  startTime: string;
  endTime: string;

  attributes: { [key: string]: number | string | boolean | null };
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

export type ResourceData = {
  attributes: { [key: string]: number | string | boolean | null };
  droppedAttributesCount: number;
};

export type ScopeData = {
  name: string;
  version: string;
  attributes: { [key: string]: number | string | boolean | null };
  droppedAttributesCount: number;
};

export type EventData = {
  name: string;
  timestamp: string;
  attributes: { [key: string]: number | string | boolean | null };
  droppedAttributeCount: number;
};

export type LinkData = {
  traceID: string;
  spanID: string;
  traceState: string;
  attributes: { [key: string]: number | string | boolean | null };
  droppedAttributesCount: number;
};
