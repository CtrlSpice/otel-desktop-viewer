import { SpanData } from "./api-types";

export enum SpanDataStatus {
  missing = "missing",
  present = "present",
}

export type SpanUIData = {
  depth: number;
  spanID: string;
  hidden: boolean;
  toggled: boolean; // controls when an element is still displayed but children are hidden
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

export type TraceSummaryWithUIData =
  | {
      hasRootSpan: true;
      rootServiceName: string;
      rootName: string;
      rootDurationString: string;
      spanCount: number;
      traceID: string;
    }
  | {
      hasRootSpan: false;
      spanCount: number;
      traceID: string;
    };

export type SidebarData = {
  numNewTraces: number;
  summaries: TraceSummaryWithUIData[];
};

export type ModifierKey = "Alt" | "Control" | "Meta" | "Shift";
