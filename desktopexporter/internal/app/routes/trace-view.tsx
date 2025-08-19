import React from "react";
import { useLoaderData } from "react-router-dom";
import { Grid, GridItem, Box, Text, VStack, Divider } from "@chakra-ui/react";

import { TraceData } from "../types/api-types";
import { telemetryAPI } from "../services/telemetry-service";
import { SpanDataStatus, SpanWithUIData } from "../types/ui-types";

import { Header } from "../components/header-view/header";
import { DetailView } from "../components/detail-view/detail-view";
import { WaterfallView } from "../components/waterfall-view/waterfall-view";
import { arrayToTree, TreeItem, RootTreeItem } from "../utils/array-to-tree";
import { getTraceBounds } from "../utils/duration";

export async function traceLoader({ params }: any) {
  try {
    // If no traceID is provided, redirect to the first available trace
    if (!params.traceID) {
      const traceSummaries = await telemetryAPI.getTraceSummaries();
      if (traceSummaries.length > 0) {
        // Redirect to the first trace
        throw new Response("", {
          status: 302,
          headers: {
            Location: `/traces/${traceSummaries[0].traceID}`,
          },
        });
      } else {
        // No traces available, redirect to root
        throw new Response("", {
          status: 302,
          headers: {
            Location: "/",
          },
        });
      }
    }

    // Load the specific trace (conversion is now handled in the API wrapper)
    const traceData = await telemetryAPI.getTraceByID(params.traceID);
    return traceData;
  } catch (error) {
    console.error('Failed to load trace:', error);
    console.error('Error details:', {
      message: error instanceof Error ? error.message : String(error),
      stack: error instanceof Error ? error.stack : undefined,
      params: params
    });
    throw error;
  }
}

export default function TraceView() {
  let traceData = useLoaderData() as TraceData;
  let traceBounds = getTraceBounds(traceData.spans);
  let spanTree: RootTreeItem[] = arrayToTree(traceData.spans);
  let orderedSpans = orderSpans(spanTree);
  
  let [selectedSpanID, setSelectedSpanID] = React.useState<string>(() => {
    if (
      !orderedSpans.length ||
      (orderedSpans[0].status === SpanDataStatus.missing &&
        orderedSpans.length < 2)
    ) {
      throw new Error("Number of spans cannot be zero");
    }

    if (orderedSpans[0].status === SpanDataStatus.missing) {
      return orderedSpans[1].metadata.spanID;
    }

    return orderedSpans[0].metadata.spanID;
  });

  // if we get a new trace because the route changed, reset the selected span
  React.useEffect(() => {
    setSelectedSpanID(
      orderedSpans[0].status === SpanDataStatus.present
        ? orderedSpans[0].metadata.spanID
        : orderedSpans[1].metadata.spanID,
    );
  }, [traceData]);

  let selectedSpan = traceData.spans.find(
    (span: { spanID: string }) => span.spanID === selectedSpanID,
  );

  return (
    <Grid
      templateAreas={`"header detail"
                       "main detail"`}
      gridTemplateColumns={"1fr 350px"}
      gridTemplateRows={"100px 1fr"}
      gap={"0"}
      height={"100vh"}
      width={"100vw"}
    >
      <GridItem area={"header"}>
        <Header traceID={traceData.traceID} />
      </GridItem>
      <GridItem
        area={"main"}
        marginLeft="20px"
      >
        <WaterfallView
          orderedSpans={orderedSpans}
          traceBounds={traceBounds}
          selectedSpanID={selectedSpanID}
          setSelectedSpanID={setSelectedSpanID}
        />
      </GridItem>
      <GridItem area={"detail"}>
        <DetailView span={selectedSpan} />
      </GridItem>
    </Grid>
  );
}

// Do a depth-first traverse of the generated tree, re-flattening it out into an array
// ordering each set of children by start time and capturing information about the
// depth of each span so we can render it correctly
//
// In the case that we are missing spans in the tree, orphaned subtrees will have a
// phantom parent span.
//
// We are sorting each set of children, but not the set of root nodes we are starting with,
// as the array-to-tree implementation is such that the root span (if one is present)
// is displayed first and all missing spans come after
function orderSpans(spanTree: RootTreeItem[]): SpanWithUIData[] {
  let orderedSpans: SpanWithUIData[] = [];

  for (let root of spanTree) {
    let stack = [
      {
        treeItem: root,
        depth: 0,
      },
    ];

    while (stack.length) {
      let node = stack.pop();
      if (!node) {
        break;
      }
      let { treeItem, depth } = node;

      if (treeItem.status === SpanDataStatus.present) {
        orderedSpans.push({
          status: SpanDataStatus.present,
          spanData: treeItem.spanData,
          metadata: { depth: depth, spanID: treeItem.spanData.spanID },
        });
      } else {
        orderedSpans.push({
          status: SpanDataStatus.missing,
          metadata: { depth: depth, spanID: treeItem.spanID },
        });
      }

      treeItem.children
        .sort((a, b) => {
          if (
            a.status === SpanDataStatus.present &&
            b.status === SpanDataStatus.present
          ) {
            // Sort by start time, with earlier times first (ascending order)
            if (a.spanData.startTime.nanoseconds < b.spanData.startTime.nanoseconds) {
              return -1;
            } else if (a.spanData.startTime.nanoseconds > b.spanData.startTime.nanoseconds) {
              return 1;
            }
            return 0;
          }
          // TODO: Throw a good error. Like, yeet it real good.
          // This doesn't happen- all missing spans are root,
          // and all children by definition have a present status
          return 0;
        })
        .forEach((child: TreeItem) =>
          stack.push({
            treeItem: child,
            depth: depth + 1,
          }),
        );
    }
  }

  return orderedSpans;
}