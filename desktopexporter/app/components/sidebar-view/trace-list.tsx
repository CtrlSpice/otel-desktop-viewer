import React, { useEffect, useRef } from "react";
import { FixedSizeList } from "react-window";
import { NavLink, useLocation, useNavigate } from "react-router-dom";
import {
  Flex,
  LinkBox,
  LinkOverlay,
  Divider,
  Text,
  useColorModeValue,
} from "@chakra-ui/react";
import { useSize } from "@chakra-ui/react-use-size";

import { TraceSummaryWithUIData } from "../../types/ui-types";
import { useKeyPress } from "../../utils/use-key-press";

const sidebarSummaryHeight = 120;
const dividerHeight = 1;

type SidebarRowData = {
  selectedTraceID: string;
  traceSummaries: TraceSummaryWithUIData[];
};

type SidebarRowProps = {
  index: number;
  style: Object;
  data: SidebarRowData;
};

function SidebarRow({ index, style, data }: SidebarRowProps) {
  let selectedColor = useColorModeValue("pink.100", "pink.900");
  let dividerColour = useColorModeValue("blackAlpha.300", "whiteAlpha.300");
  let { selectedTraceID, traceSummaries } = data;
  let traceSummary = traceSummaries[index];

  let isSelected =
    selectedTraceID && selectedTraceID === traceSummary.traceID ? true : false;

  let backgroundColour = isSelected ? selectedColor : "";

  if (traceSummary.hasRootSpan) {
    // Add zero-width space after forward slashes, dashes, and dots
    // to indicate line breaking opportunity
    let rootNameLabel = traceSummary.rootName
      .replaceAll("/", "/\u200B")
      .replaceAll("-", "-\u200B")
      .replaceAll(".", ".\u200B");

    let rootServiceNameLabel = traceSummary.rootServiceName
      .replaceAll("/", "/\u200B")
      .replaceAll("-", "-\u200B")
      .replaceAll(".", ".\u200B");

    return (
      <div style={style}>
        <Divider
          height={dividerHeight}
          borderColor={dividerColour}
        />
        <LinkBox
          display="flex"
          flexDirection="column"
          justifyContent="center"
          bgColor={backgroundColour}
          height={`${sidebarSummaryHeight}px`}
          paddingX="20px"
        >
          <Text
            fontSize="xs"
            noOfLines={1}
          >
            {"Root Service Name: "}
            <strong>{rootServiceNameLabel}</strong>
          </Text>
          <Text
            fontSize="xs"
            noOfLines={2}
          >
            {"Root Name: "}
            <strong>{rootNameLabel}</strong>
          </Text>
          <Text fontSize="xs">
            {"Root Duration: "}
            <strong>{traceSummary.rootDurationString}</strong>
          </Text>
          <Text fontSize="xs">
            {"Number of Spans: "}
            <strong>{traceSummary.spanCount}</strong>
          </Text>
          <LinkOverlay
            as={NavLink}
            to={`traces/${traceSummary.traceID}`}
          >
            <Text fontSize="xs">
              {"Trace ID: "}
              <strong>{traceSummary.traceID}</strong>
            </Text>
          </LinkOverlay>
        </LinkBox>
      </div>
    );
  }

  return (
    <div style={style}>
      <Divider
        height={dividerHeight}
        borderColor={dividerColour}
      />
      <LinkBox
        display="flex"
        flexDirection="column"
        justifyContent="center"
        bgColor={backgroundColour}
        height={`${sidebarSummaryHeight}px`}
        paddingX="20px"
      >
        <Text fontSize="xs">
          {"Incomplete Trace: "}
          <strong>{"missing a root span"}</strong>
        </Text>
        <Text fontSize="xs">
          {"Number of Spans: "}
          <strong>{traceSummary.spanCount}</strong>
        </Text>
        <LinkOverlay
          as={NavLink}
          to={`traces/${traceSummary.traceID}`}
        >
          <Text fontSize="xs">
            {"Trace ID: "}
            <strong>{traceSummary.traceID}</strong>
          </Text>
        </LinkOverlay>
      </LinkBox>
    </div>
  );
}

type TraceListProps = {
  traceSummaries: TraceSummaryWithUIData[];
};

export function TraceList(props: TraceListProps) {
  let ref = useRef(null);
  let size = useSize(ref);

  let location = useLocation();
  let navigate = useNavigate();

  let selectedIndex = 0;
  let selectedTraceID = "";
  let { traceSummaries } = props;

  // Default to the first trace in the list if none are selected
  if (location.pathname.includes("/traces/")) {
    selectedTraceID = location.pathname.split("/")[2];
    selectedIndex = traceSummaries.findIndex(
      (summary) => summary.traceID === selectedTraceID,
    );
  } else {
    selectedTraceID = traceSummaries[selectedIndex].traceID;
    window.location.href = `/traces/${selectedTraceID}`;
  }

  // Set up keyboard navigation
  let arrowLeftPressed = useKeyPress("ArrowLeft");
  let arrowRightPressed = useKeyPress("ArrowRight");
  let hPressed = useKeyPress("h");
  let lPressed = useKeyPress("l");

  useEffect(() => {
    if (arrowLeftPressed || hPressed) {
      selectedIndex = selectedIndex > 0 ? selectedIndex - 1 : 0;
      selectedTraceID = traceSummaries[selectedIndex].traceID;
      navigate(`/traces/${selectedTraceID}`);
    }
  }, [arrowLeftPressed, hPressed]);

  useEffect(() => {
    if (arrowRightPressed || lPressed) {
      selectedIndex =
        selectedIndex < traceSummaries.length - 1
          ? selectedIndex + 1
          : traceSummaries.length - 1;
      selectedTraceID = traceSummaries[selectedIndex].traceID;
      navigate(`/traces/${selectedTraceID}`);
    }
  }, [arrowRightPressed, lPressed]);

  let itemData = {
    selectedTraceID: selectedTraceID,
    traceSummaries: traceSummaries,
  };

  let itemHeight = sidebarSummaryHeight + dividerHeight;

  return (
    <Flex
      ref={ref}
      height="100%"
    >
      <FixedSizeList
        height={size ? size.height : 0}
        itemData={itemData}
        itemCount={props.traceSummaries.length}
        itemSize={itemHeight}
        width="100%"
      >
        {SidebarRow}
      </FixedSizeList>
    </Flex>
  );
}
