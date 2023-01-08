import { SpanData } from "./api-types";

export type SpanMetaData = {
  depth: number;
};

export type SpanWithMetadata = {
  spanID: string;
  spanData: SpanData | null;
  metadata: SpanMetaData;
};
