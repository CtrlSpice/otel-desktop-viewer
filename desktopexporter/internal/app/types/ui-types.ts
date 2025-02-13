import { SpanData, TraceSummary } from "./api-types";

export enum SpanDataStatus {
  missing = "missing",
  present = "present",
}

export type SpanUIData = {
  depth: number;
  spanID: string;
};

export type SpanWithUIData =
  | {
      status: SpanDataStatus.present;
      spanData: SpanData;
      metadata: SpanUIData;
    }
  | {
      status: SpanDataStatus.missing;
      metadata: SpanUIData;
    };

export type RootSpanWithUIData = {
  serviceName: string;
  name: string;
  durationString: string;
};

export type TraceSummaryWithUIData = {
  root?: RootSpanWithUIData;
  spanCount: number;
};

export type SidebarData = {
  numNewTraces: number;
  summaries: Map<string, TraceSummaryWithUIData>;
};

export type ModifierKey = "Alt" | "Control" | "Meta" | "Shift";