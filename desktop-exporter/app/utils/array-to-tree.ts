import { SpanData } from "../types/api-types";
import { SpanDataStatus } from "../types/ui-types";

export type TreeItem =
  | {
      status: SpanDataStatus.present;
      spanData: SpanData;
      children: TreeItem[];
    }
  | {
      status: SpanDataStatus.missing;
      spanID: string;
      children: TreeItem[];
    };

export function arrayToTree(spans: SpanData[]): TreeItem[] {
  let rootItems: TreeItem[] = [];
  let lookup: { [spanID: string]: TreeItem } = {};
  let missingSpanIDs: Set<string> = new Set();

  for (let spanData of spans) {
    let { spanID, parentSpanID } = spanData;

    // If the span is not in the lookup structure yet, add it
    if (!lookup[spanID]) {
      lookup[spanID] = {
        status: SpanDataStatus.present,
        spanData: spanData,
        children: [],
      };
    }

    // Note A:
    // If the span has been added to the lookup structure as a missing/incomplete parent
    // on a previous pass (see Note B), update it and mark it present, and remove it from the missing set.
    if (lookup[spanID].status === SpanDataStatus.missing) {
      let children = lookup[spanID].children;
      lookup[spanID] = {
        status: SpanDataStatus.present,
        spanData: spanData,
        children: children,
      };
      missingSpanIDs.delete(spanID);
    }

    let treeItem = lookup[spanID];

    // If the current span has no parentSpanID, add it to the rootItems
    if (!parentSpanID) {
      rootItems.push(treeItem);
    } else {
      // Note B:
      // If the current span's parentSpanID is not in the lookup structure yet, add it
      // as a missing/incomplete span, to be updated in a subsequent loop if found (see note A)
      if (!lookup[parentSpanID]) {
        lookup[parentSpanID] = {
          status: SpanDataStatus.missing,
          spanID: parentSpanID,
          children: [],
        };
        // Add the partial span to the missing set.
        missingSpanIDs.add(parentSpanID);
      }

      // Finally, add the current span to its parent's children array.
      lookup[parentSpanID].children.push(treeItem);
    }
  }

  // In order to handle incomplete traces, the missing spans get appended to the end of the rootItems array
  // This way we make sure that the root span is added first (if present), followed by any missing spans
  for (let spanID of missingSpanIDs) {
    rootItems.push(lookup[spanID]);
  }

  return rootItems;
}
