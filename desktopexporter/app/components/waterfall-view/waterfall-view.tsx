import React, { useRef, ClipboardEvent, useEffect } from "react";
import { FixedSizeList } from "react-window";
import { Flex } from "@chakra-ui/react";
import { useSize } from "@chakra-ui/react-use-size";

import { SpanDataStatus, SpanWithUIData } from "../../types/ui-types";
import { WaterfallRow } from "./waterfall-row";
import { HeaderRow } from "./header-row";
import { TraceTiming } from "../../utils/duration";
import { useKeyPress } from "../../utils/use-key-press";

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

  let { orderedSpans, traceTimeAttributes, selectedSpanID, setSelectedSpanID } =
    props;

  // Set up keyboard navigation
  let arrowUpPressed = useKeyPress("ArrowUp");
  let arrowDownPressed = useKeyPress("ArrowDown");
  let kPressed = useKeyPress("k");
  let jPressed = useKeyPress("j");

  let selectedIndex = orderedSpans.findIndex(
    (span) => span.metadata.spanID === selectedSpanID,
  );
  let firstSelectableIndex = orderedSpans.findIndex(
    (span) => span.status === SpanDataStatus.present,
  );

  useEffect(() => {
    if (arrowUpPressed || kPressed) {
      // Move up while skipping the missing spans in incomplete traces
      if (selectedIndex > firstSelectableIndex) {
        do {
          selectedIndex--;
        } while (orderedSpans[selectedIndex].status === SpanDataStatus.missing);
        setSelectedSpanID(orderedSpans[selectedIndex].metadata.spanID);
      }
    }
  }, [arrowUpPressed, kPressed]);

  useEffect(() => {
    if (arrowDownPressed || jPressed) {
      // Move down while skipping the missing spans in incomplete traces
      if (selectedIndex < orderedSpans.length - 1) {
        do {
          selectedIndex++;
        } while (orderedSpans[selectedIndex].status === SpanDataStatus.missing);
        setSelectedSpanID(orderedSpans[selectedIndex].metadata.spanID);
      }
    }
  }, [arrowDownPressed, jPressed]);

  let rowData = {
    orderedSpans: orderedSpans,
    traceTimeAttributes: traceTimeAttributes,
    spanNameColumnWidth: spanNameColumnWidth,
    serviceNameColumnWidth: serviceNameColumnWidth,
    selectedSpanID: selectedSpanID,
    setSelectedSpanID: setSelectedSpanID,
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
