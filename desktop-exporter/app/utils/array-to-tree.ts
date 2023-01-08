import { SpanData } from "../types/api-types";

export interface TreeItem {
  span: { spanID: string; spanData: SpanData | null };
  children: TreeItem[];
}

export function arrayToTree(spans: SpanData[]): TreeItem[] {
  let rootItems: TreeItem[] = [];
  let lookup: { [spanID: string]: TreeItem } = {};
  let missingSpanIDs: Set<string> = new Set();

  for (let span of spans) {
    let { spanID, parentSpanID } = span;

    if (!lookup[spanID]) {
      lookup[spanID] = {
        span: { spanID: spanID, spanData: span },
        children: [],
      };
    } else if (!lookup[spanID].span.spanData) {
      lookup[spanID].span.spanData = span;
    }

    missingSpanIDs.delete(spanID);

    let treeItem = lookup[spanID];

    if (!parentSpanID) {
      rootItems.push(treeItem);
    } else {
      if (!lookup[parentSpanID]) {
        lookup[parentSpanID] = {
          span: { spanID: parentSpanID, spanData: null },
          children: [],
        };
        missingSpanIDs.add(parentSpanID);
      }
      lookup[parentSpanID].children.push(treeItem);
    }
  }

  for (let spanID of missingSpanIDs) {
    rootItems.push(lookup[spanID]);
  }

  return rootItems;
}
