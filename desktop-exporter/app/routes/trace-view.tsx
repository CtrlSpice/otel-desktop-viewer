import React from "react";
import { useLoaderData } from "react-router-dom";
import { Grid, GridItem } from "@chakra-ui/react";

import { Header } from "../components/header";
import { DetailView } from "../components/detail-view/detail-view";
import { WaterfallView } from "../components/waterfall-view/waterfall-view";
import { TraceData } from "../types/api-types";

export async function traceLoader({ params }: any) {
  const response = await fetch(`/api/traces/${params.traceID}`);
  const traceData = await response.json();
  return traceData;
}



export default function TraceView() {
  const traceData = useLoaderData() as TraceData;
  const [selectedSpanID, setSelectedSpanID] = React.useState<string>(
    traceData.spans.length ? traceData.spans[0].spanID : "",
  );

  // if we get a new trace because the route changed, reset the selected span
  React.useEffect(() => {
    setSelectedSpanID(traceData.spans[0].spanID);
  }, [traceData]);

  const selectedSpan = traceData.spans.find(
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
          spans={traceData.spans}
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
