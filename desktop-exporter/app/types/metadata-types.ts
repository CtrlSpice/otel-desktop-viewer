import { SpanData } from "./api-types";

export type SpanMetaData = {
  depth: number;
};

export type SpanWithMetadata = {
  span: SpanData;
  metadata: SpanMetaData;
};
