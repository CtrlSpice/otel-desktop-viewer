import React from "react";
import { useLoaderData } from "react-router-dom";
import { Grid, GridItem } from "@chakra-ui/react";

import { TraceData } from "../types/api-types";
import { SpanWithMetadata } from "../types/metadata-types";

import { Header } from "../components/header";
import { DetailView } from "../components/detail-view/detail-view";
import { WaterfallView } from "../components/waterfall-view/waterfall-view";
import { arrayToTree, TreeItem } from "../utils/array-to-tree";
import { getNsFromString, getTraceDurationNs } from "../utils/duration";

export async function traceLoader({ params }: any) {
  let response = await fetch(`/api/traces/${params.traceID}`);
  let traceData = await response.json();
  return traceData;
}

export default function TraceView() {
  let traceData = useLoaderData() as TraceData;
  let traceDurationNs = getTraceDurationNs(traceData.spans);
  let orderedSpans: SpanWithMetadata[] = [];
  let spanTree: TreeItem[] = [];

  spanTree = arrayToTree(traceData.spans);

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

      orderedSpans.push({
        spanID: treeItem.span.spanID,
        spanData: treeItem.span.spanData,
        metadata: { depth: node.depth },
      });

      treeItem.children
        .sort((a, b) => {
          let aStart = a.span.spanData
            ? getNsFromString(a.span.spanData.startTime)
            : 0;
          let bStart = b.span.spanData
            ? getNsFromString(b.span.spanData.startTime)
            : 0;

          if ((aStart = 0)) {
            return bStart - aStart;
          }
          return aStart - bStart;
        })
        .forEach((child: TreeItem) =>
          stack.push({
            treeItem: child,
            depth: depth + 1,
          }),
        );
    }
  }

  let [selectedSpanID, setSelectedSpanID] = React.useState<string>(
    orderedSpans.length ? orderedSpans[0].spanID : "",
  );

  // if we get a new trace because the route changed, reset the selected span
  React.useEffect(() => {
    setSelectedSpanID(orderedSpans[0].spanID);
  }, [traceData]);

  let selectedSpan = traceData.spans.find(
    (span: { spanID: string }) => span.spanID === selectedSpanID,
  );

  return (
    <Grid
      templateAreas={`"header header"
                       "main detail"`}
      gridTemplateColumns={"1fr 350px"}
      gridTemplateRows={"60px 1fr"}
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
          traceDurationNs={traceDurationNs}
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