import { SpanData } from "./api-types";

export enum SpanDataStatus {
  missing,
  present,
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
