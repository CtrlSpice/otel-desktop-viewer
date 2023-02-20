import { SpanData } from "../types/api-types";
import { SpanDataStatus } from "../types/ui-types";
import { getNsFromString } from "./duration";

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

  // To handle incomplete traces:
  // 1. Sort the missing spans by the earliest start time of their children
  // 2. Appended sorted spand to the end of the rootItems array
  // This way we make sure that the root span is added first (if present),
  // and preserve the visual clarity of the waterfall view
  let missingSpans = Array.from(missingSpanIDs).sort((a: string, b: string) => {
    if (lookup[a].children.length < 1 || lookup[b].children.length < 1) {
      // This should logically never happen in this implementation, since a missing span
      // must have at least one child with its spanID as a parentSpanID in order to be created.
      throw new Error(
        "Unexpected type: A 'missing' parent span appears to have no children.",
      );
    }
    let childrenA = lookup[a].children;
    let childrenB = lookup[b].children;

    let earliestStartTimeA = 0;
    let earliestStartTimeB = 0;

    if (
      childrenA[0].status === SpanDataStatus.present &&
      childrenB[0].status === SpanDataStatus.present
    ) {
      earliestStartTimeA = getNsFromString(childrenA[0].spanData.startTime);
      earliestStartTimeB = getNsFromString(childrenB[0].spanData.startTime);
    } else {
      // This should also logically never happen in this implementation, since that the child span
      // must have SpanData (minimally a parentSpanID) in order for the parent span to be created.
      throw new Error(
        "Unexpected type: A child of a 'missing' parent span appears to have no SpanData.",
      );
    }

    // Find the earliest start time for A
    childrenA.forEach((treeItem) => {
      if (treeItem.status === SpanDataStatus.missing) {
        // Again, should logically never happen in this implementation, since that the child span
        // must have SpanData (minimally a parentSpanID) in order for the parent span to be created.
        throw new Error(
          "Unexpected type: A child of a 'missing' parent span appears to have no SpanData.",
        );
      }
      let spanStart = getNsFromString(treeItem.spanData.startTime);
      if (spanStart < earliestStartTimeA) {
        earliestStartTimeA = spanStart;
      }
    });

    // Find the earliest start time for B
    childrenB.forEach((treeItem) => {
      if (treeItem.status === SpanDataStatus.missing) {
        // Again, should logically never happen in this implementation, since that the child span
        // must have SpanData (minimally a parentSpanID) in order for the parent span to be created.
        // This is the last one, I swear.
        throw new Error(
          "Unexpected type: A child of a 'missing' parent span appears to have no SpanData.",
        );
      }
      let spanStart = getNsFromString(treeItem.spanData.startTime);
      if (spanStart < earliestStartTimeB) {
        earliestStartTimeB = spanStart;
      }
    });

    return earliestStartTimeA - earliestStartTimeB;
  });

  for (let spanID of missingSpans) {
    rootItems.push(lookup[spanID]);
  }

  return rootItems;
}
