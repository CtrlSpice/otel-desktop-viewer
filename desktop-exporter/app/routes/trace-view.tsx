import React from "react";
import { useLoaderData } from "react-router-dom";
import { FixedSizeList } from "react-window";

import { Header } from "../components/header";
import { SpanData, TraceData } from "../types/api-types";

export async function traceLoader({ params }: any) {
  const response = await fetch(`/api/traces/${params.traceID}`);
  const traceData = await response.json();
  return traceData;
}

type WaterfallRowProps = {
  index: number;
  style: {};
  data: WaterfallViewProps;
};

function WaterfallRow({ index, style, data }: WaterfallRowProps) {
  let { spans, selectedSpanID, setSelectedSpanID } = data;
  let span = spans[index];

  let className = "waterfall-item";
  className += index % 2 ? " odd" : " even";
  if (!!selectedSpanID) {
    className += span.spanID === selectedSpanID ? " active" : "";
  }

  return (
    <div
      className={className}
      style={style}
      onClick={() => setSelectedSpanID(span.spanID)}
    >
      Name: {span.name} SpanID: {span.spanID}
    </div>
  );
}

type WaterfallViewProps = {
  spans: SpanData[];
  selectedSpanID: string | undefined;
  setSelectedSpanID: (spanID: string) => void;
};

function WaterfallView(props: WaterfallViewProps) {
  return (
    <FixedSizeList
      className="List"
      height={300}
      itemData={props}
      itemCount={props.spans.length}
      itemSize={30}
      width={"100%"}
    >
      {WaterfallRow}
    </FixedSizeList>
  );
}

type DetailViewProps = {
  span: SpanData | undefined;
};

function DetailView(props: DetailViewProps) {
  let { span } = props;
  if (!span) {
    return <div className="detail"></div>;
  }
  return (
    <div className="detail">
      <pre>{`
Name: ${span.name}
Kind: ${span.kind}
Start: ${span.startTime}
End: ${span.endTime}
    `}</pre>
    </div>
  );
}

export default function TraceView() {
  const traceData = useLoaderData() as TraceData;
  const [selectedSpanID, setSelectedSpanID] = React.useState<
    string | undefined
  >(traceData.spans[0].spanID);

  // if we get a new trace because the route changed, reset the selected span
  React.useEffect(() => {
    setSelectedSpanID(traceData.spans[0].spanID);
  }, [traceData]);

  const selectedSpan = traceData.spans.find(
    (span) => span.spanID === selectedSpanID,
  );

  return (
    <div className="traceview">
      <Header traceID={traceData.traceID} />
      <WaterfallView
        spans={traceData.spans}
        selectedSpanID={selectedSpanID}
        setSelectedSpanID={setSelectedSpanID}
      />
      <DetailView span={selectedSpan} />
    </div>
  );
}
