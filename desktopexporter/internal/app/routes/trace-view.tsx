import React from "react";
import { useLoaderData } from "react-router-dom";
import { Grid, GridItem, Box, Text, VStack, Divider } from "@chakra-ui/react";

import { TraceData } from "../types/api-types";
import { telemetryAPI } from "../services/telemetry-service";
import { SpanDataStatus, SpanWithUIData } from "../types/ui-types";

import { Header } from "../components/header-view/header";
import { DetailView } from "../components/detail-view/detail-view";
import { WaterfallView } from "../components/waterfall-view/waterfall-view";
import { getTraceBounds } from "../utils/duration";

export async function traceLoader({ params }: any) {
  try {
    // If no traceID is provided, redirect to the first available trace
    if (!params.traceID) {
      let traceSummaries = await telemetryAPI.getTraceSummaries();
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

    // Load the specific trace
    let traceData = await telemetryAPI.getTraceByID(params.traceID);
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
  let traceBounds = getTraceBounds(traceData.spans.map(spanNode => spanNode.spanData));
  
  // Convert the new structure to the format expected by the UI components
  let orderedSpans: SpanWithUIData[] = traceData.spans.map(spanNode => ({
    status: SpanDataStatus.present,
    spanData: spanNode.spanData,
    metadata: { 
      depth: spanNode.depth, 
      spanID: spanNode.spanData.spanID 
    }
  }));
  
  let [selectedSpanID, setSelectedSpanID] = React.useState<string>(() => {
    if (!orderedSpans.length) {
      throw new Error("Number of spans cannot be zero");
    }
    return orderedSpans[0].metadata.spanID;
  });

  // if we get a new trace because the route changed, reset the selected span
  React.useEffect(() => {
    setSelectedSpanID(orderedSpans[0].metadata.spanID);
  }, [traceData]);

  let selectedSpan = traceData.spans.find(
    (spanNode) => spanNode.spanData.spanID === selectedSpanID,
  )?.spanData;

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

