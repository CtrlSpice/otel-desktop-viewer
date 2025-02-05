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

  attributes: Attribute[];
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
  attributes: Attribute[];
  droppedAttributesCount: number;
};

export type ScopeData = {
  name: string;
  version: string;
  attributes: Attribute[];
  droppedAttributesCount: number;
};

export type EventData = {
  name: string;
  timestamp: string;
  attributes: Attribute[];
  droppedAttributesCount: number;
};

export type LinkData = {
  traceID: string;
  spanID: string;
  traceState: string;
  attributes: Attribute[];
  droppedAttributesCount: number;
};

export type Attribute = {
  [key: string]:
    | string
    | number
    | boolean
    | string[]
    | number[]
    | boolean[];
};
