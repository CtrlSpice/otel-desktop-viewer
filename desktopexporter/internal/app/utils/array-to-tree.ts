import { SpanData } from "../types/api-types";
import { SpanDataStatus } from "../types/ui-types";
import { PreciseTimestamp } from "../types/precise-timestamp";

export type TreeItem = {
  status: SpanDataStatus.present;
  spanData: SpanData;
  children: TreeItem[];
};

export type MissingTreeItem = {
  status: SpanDataStatus.missing;
  spanID: string;
  children: TreeItem[];
};

export type RootTreeItem =
  | TreeItem
  | MissingTreeItem;

export function arrayToTree(spans: SpanData[]): RootTreeItem[] {
  let rootItems: RootTreeItem[] = [];
  let lookup: { [spanID: string]: RootTreeItem } = {};
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

    let treeItem = lookup[spanID];

    // Note A:
    // If the span has been added to the lookup structure as a missing/incomplete parent
    // on a previous pass (see Note B), update it and mark it present, and remove it from the missing set.
    if (treeItem.status === SpanDataStatus.missing) {
      // Re-assign treeItem as a TreeItem type so that after this if statement,
      // the type system knows that treeItem can only be a TreeItem
      treeItem = {
        status: SpanDataStatus.present,
        spanData: spanData,
        children: treeItem.children,
      };
      // overwrite the stored version since now we know it is present
      lookup[spanID] = treeItem
      missingSpanIDs.delete(spanID);
    }

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
  let missingIDsArray = Array.from(missingSpanIDs).sort(
    (a: string, b: string) => {
      let earliestStartTimeA = getEarliestStartTime(lookup[a].children);
      let earliestStartTimeB = getEarliestStartTime(lookup[b].children);
      
      if (earliestStartTimeA.nanoseconds < earliestStartTimeB.nanoseconds) {
        return -1;
      } else if (earliestStartTimeA.nanoseconds > earliestStartTimeB.nanoseconds) {
        return 1;
      }
      return 0;
    },
  );

  for (let spanID of missingIDsArray) {
    rootItems.push(lookup[spanID]);
  }

  return rootItems;
}

function getEarliestStartTime(children: TreeItem[]): PreciseTimestamp {
  if (children.length == 0) {
    // This should logically never happen in this implementation, since a missing span
    // must have at least one child with its spanID as a parentSpanID in order to be created.
    throw new Error(
      "Unexpected type: A 'missing' parent span appears to have no children.",
    );
  }

  let earliestStart = children[0].spanData.startTime;
  for (let i = 1; i < children.length; i++) {
    let currentStart = children[i].spanData.startTime;
    if (currentStart.nanoseconds < earliestStart.nanoseconds) {
      earliestStart = currentStart;
    }
  }

  return earliestStart;
}