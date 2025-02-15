import { RootSpan, SpanData } from "./api-types";

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
    
export type TraceSummaryWithUIData = {
  root?: RootSpan;
  spanCount: number;
};

export type SidebarData = {
  numNewTraces: number;
  summaries: Map<string, TraceSummaryWithUIData>;
};

export type ModifierKey = "Alt" | "Control" | "Meta" | "Shift";