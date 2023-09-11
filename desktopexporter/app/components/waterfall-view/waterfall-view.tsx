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
  toggle: (id: string) => void;
};

export function WaterfallView(props: WaterfallViewProps) {
  let containerRef = useRef(null);
  let spanListRef = React.createRef<FixedSizeList>();
  const size = useSize(containerRef);

  const waterfallItemHeight = 50;
  const headerRowHeight = 30;
  const spanNameColumnWidth = 300;
  const serviceNameColumnWidth = 200;

  let { orderedSpans, traceTimeAttributes, selectedSpanID, setSelectedSpanID, toggle } =
    props;

  // Set up keyboard navigation
  let prevSpanKeyPressed = useKeyPress(["ArrowUp", "k"]);
  let nextSpanKeyPressed = useKeyPress(["ArrowDown", "j"]);

  let selectedIndex = orderedSpans.findIndex(
    (span) => span.metadata.spanID === selectedSpanID,
  );
  let firstSelectableIndex = orderedSpans.findIndex(
    (span) => span.status === SpanDataStatus.present,
  );

  useEffect(() => {
    if (prevSpanKeyPressed) {
      // Move up while skipping the missing spans in incomplete traces
      if (selectedIndex > firstSelectableIndex) {
        do {
          selectedIndex--;
        } while (orderedSpans[selectedIndex].status === SpanDataStatus.missing);
        setSelectedSpanID(orderedSpans[selectedIndex].metadata.spanID);
        spanListRef.current?.scrollToItem(selectedIndex);
      }
    }
  }, [prevSpanKeyPressed]);

  useEffect(() => {
    if (nextSpanKeyPressed) {
      // Move down while skipping the missing spans in incomplete traces
      if (selectedIndex < orderedSpans.length - 1) {
        do {
          selectedIndex++;
        } while (orderedSpans[selectedIndex].status === SpanDataStatus.missing);
        setSelectedSpanID(orderedSpans[selectedIndex].metadata.spanID);
        spanListRef.current?.scrollToItem(selectedIndex);
      }
    }
  }, [nextSpanKeyPressed]);

  // filter out any spans that are hidden
  orderedSpans = orderedSpans.filter((span) => !span.metadata.hidden)

  let rowData = {
    orderedSpans,
    traceTimeAttributes,
    spanNameColumnWidth,
    serviceNameColumnWidth,
    selectedSpanID,
    setSelectedSpanID,
    toggle,
  };

  return (
    <Flex
      direction="column"
      ref={containerRef}
      height="100%"
      onCopy={stripZeroWidthSpacesOnCopyCallback}
    >
      <HeaderRow
        headerRowHeight={headerRowHeight}
        spanNameColumnWidth={spanNameColumnWidth}
        serviceNameColumnWidth={serviceNameColumnWidth}
        traceDuration={traceTimeAttributes.traceDurationNS}
      />
      <FixedSizeList
        className="List"
        height={size ? size.height - headerRowHeight : 0}
        itemData={rowData}
        itemCount={orderedSpans.length}
        itemSize={waterfallItemHeight}
        ref={spanListRef}
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
