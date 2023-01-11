import React, { useRef, ClipboardEvent } from "react";
import { FixedSizeList } from "react-window";
import { Flex } from "@chakra-ui/react";
import { useSize } from "@chakra-ui/react-use-size";

import { SpanWithUIData } from "../../types/metadata-types";
import { WaterfallRow } from "./waterfall-row";
import { HeaderRow } from "./header-row";
import { TraceTiming } from "../../utils/duration";

type WaterfallViewProps = {
  orderedSpans: SpanWithUIData[];
  traceTimeAttributes: TraceTiming;
  selectedSpanID: string | undefined;
  setSelectedSpanID: (spanID: string) => void;
};

export function WaterfallView(props: WaterfallViewProps) {
  const ref = useRef(null);
  const size = useSize(ref);

  const waterfallItemHeight = 50;
  const headerRowHeight = 30;
  const spanNameColumnWidth = 300;
  const serviceNameColumnWidth = 200;

  let rowData = {
    orderedSpans: props.orderedSpans,
    traceTimeAttributes: props.traceTimeAttributes,
    spanNameColumnWidth: spanNameColumnWidth,
    serviceNameColumnWidth: serviceNameColumnWidth,
    selectedSpanID: props.selectedSpanID,
    setSelectedSpanID: props.setSelectedSpanID,
  };

  return (
    <Flex
      direction="column"
      ref={ref}
      height="100%"
      onCopy={stripZeroWidthSpacesOnCopyCallback}
    >
      <HeaderRow
        headerRowHeight={headerRowHeight}
        spanNameColumnWidth={spanNameColumnWidth}
        serviceNameColumnWidth={serviceNameColumnWidth}
        traceDuration={props.traceTimeAttributes.traceDurationNS}
      />
      <FixedSizeList
        className="List"
        height={size ? size.height - headerRowHeight : 0}
        itemData={rowData}
        itemCount={props.orderedSpans.length}
        itemSize={waterfallItemHeight}
        width={"100%"}
      >
        {WaterfallRow}
      </FixedSizeList>
    </Flex>
  );
}

function stripZeroWidthSpacesOnCopyCallback(
  e: ClipboardEvent<HTMLParagraphElement>,
) {
  let selection = window.getSelection();
  if (!selection) {
    return;
  }
  let text = selection.toString().replaceAll("\u200B", "");
  e.clipboardData?.setData("text/plain", text);
  e.preventDefault();
}

