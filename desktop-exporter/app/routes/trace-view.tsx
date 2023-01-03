import React from "react";
import { useLoaderData } from "react-router-dom";
import { Grid, GridItem } from "@chakra-ui/react";
import { arrayToTree, TreeItem } from "performant-array-to-tree";

import { Header } from "../components/header";
import { DetailView } from "../components/detail-view/detail-view";
import { WaterfallView } from "../components/waterfall-view/waterfall-view";
import { getNsFromString } from "../utils/duration";
import { TraceData, SpanData } from "../types/api-types";

export async function traceLoader({ params }: any) {
  let response = await fetch(`/api/traces/${params.traceID}`);
  let traceData = await response.json();
  return traceData;
}

export default function TraceView() {
  let traceData = useLoaderData() as TraceData;
  let spanTree = arrayToTree(traceData.spans, {
    id: "spanID",
    parentId: "parentSpanID",
  });
  let orderedSpans = orderSpans(spanTree[0], 0, []);
  let [selectedSpanID, setSelectedSpanID] = React.useState<string>(
    orderedSpans.length ? orderedSpans[0].spanID : "",
  );

  // if we get a new trace because the route changed, reset the selected span
  React.useEffect(() => {
    setSelectedSpanID(orderedSpans[0].spanID);
  }, [traceData]);

  let selectedSpan = traceData.spans.find(
    (span) => span.spanID === selectedSpanID,
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
          spans={orderedSpans}
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

function orderSpans(
  parentNode: TreeItem,
  depth: number,
  orderedSpans: SpanData[],
) {
  let parentSpan = parentNode.data;
  parentSpan.depth = depth;
  orderedSpans.push(parentSpan);

  let children = parentNode.children.sort(
    // Not sure if this is how you write this in typescript, but I did my best
    (a: { data: { startTime: string } }, b: { data: { startTime: string } }) =>
      getNsFromString(a.data.startTime) - getNsFromString(b.data.startTime),
  );

  children.forEach((node: TreeItem) => {
    orderedSpans = orderSpans(node, depth + 1, orderedSpans);
  });

  return orderedSpans;
}