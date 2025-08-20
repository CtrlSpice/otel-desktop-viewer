import React, { useEffect, useRef } from "react";
import { FixedSizeList } from "react-window";
import { NavLink, useLocation, useNavigate } from "react-router-dom";
import {
  Flex,
  LinkBox,
  LinkOverlay,
  Divider,
  Text,
  Button,
  useColorModeValue,
  useDisclosure,
} from "@chakra-ui/react";
import { useSize } from "@chakra-ui/react-use-size";

import { TraceSummaryWithUIData } from "../../types/ui-types";
import { useKeyCombo, useKeyPress } from "../../utils/use-key-press";
import { KeyboardHelp } from "../modals/keyboard-help";
import { formatDuration } from "../../utils/duration";
import { telemetryAPI } from "../../services/telemetry-service";

const sidebarSummaryHeight = 120;
const dividerHeight = 1;

type SidebarRowData = {
  selectedTraceID: string;
  traceSummaries: Map<string, TraceSummaryWithUIData>;
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

  // Get traceID for this index
  let traceID = Array.from(traceSummaries.keys())[index];
  let traceSummary = traceSummaries.get(traceID)!;
  let isSelected = selectedTraceID === traceID;
  let backgroundColour = isSelected ? selectedColor : "";

  if (traceSummary.root) {
    // Add zero-width space after forward slashes, dashes, and dots
    // to indicate line breaking opportunity
    let rootNameLabel = traceSummary.root.name
      .replaceAll("/", "/\u200B")
      .replaceAll("-", "-\u200B")
      .replaceAll(".", ".\u200B");

    let rootServiceNameLabel = traceSummary.root.serviceName
      .replaceAll("/", "/\u200B")
      .replaceAll("-", "-\u200B")
      .replaceAll(".", ".\u200B");

    let duration = traceSummary.root.endTime.nanoseconds - traceSummary.root.startTime.nanoseconds;

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
            <strong>{formatDuration(duration)}</strong>
          </Text>
          <Text fontSize="xs">
            {"Number of Spans: "}
            <strong>{traceSummary.spanCount}</strong>
          </Text>
          <LinkOverlay
            as={NavLink}
            to={`traces/${traceID}`}
          >
            <Text fontSize="xs">
              {"Trace ID: "}
              <strong>{traceID}</strong>
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
          to={`traces/${traceID}`}
        >
          <Text fontSize="xs">
            {"Trace ID: "}
            <strong>{traceID}</strong>
          </Text>
        </LinkOverlay>
      </LinkBox>
    </div>
  );
}

type TraceListProps = {
  traceSummaries: Map<string, TraceSummaryWithUIData>;
};

export function TraceList(props: TraceListProps) {
  let containerRef = useRef(null);
  let summaryListRef = React.createRef<FixedSizeList>();
  let size = useSize(containerRef);

  let location = useLocation();
  let navigate = useNavigate();

  let { isOpen, onOpen, onClose } = useDisclosure();

  // Feature flag checks
  let enableLogs = localStorage.getItem('enableLogs') === 'true';
  let enableMetrics = localStorage.getItem('enableMetrics') === 'true';

  let selectedIndex = 0;
  let selectedTraceID = "";
  let { traceSummaries } = props;

  // Convert to array only for indexing operations
  const traceIDs = Array.from(traceSummaries.keys());

  // Default to the first trace in the list if none are selected
  if (location.pathname.includes("/traces/")) {
    selectedTraceID = location.pathname.split("/")[2];
    selectedIndex = traceIDs.indexOf(selectedTraceID);
  } else if (!location.pathname.includes("/logs") && !location.pathname.includes("/metrics") && traceIDs.length > 0) {
    // Only redirect if we're not on the logs or metrics page and have traces
    selectedTraceID = traceIDs[selectedIndex];
    window.location.href = `/traces/${selectedTraceID}`;
  }

  // Scroll to the currently selected trace summary on load
  useEffect(() => {
    summaryListRef.current?.scrollToItem(selectedIndex, "start");
  }, []);

  // Set up keyboard navigation
  let prevTraceKeyPressed = useKeyPress(["ArrowLeft", "h"]);
  let nextTraceKeyPressed = useKeyPress(["ArrowRight", "l"]);
  let reloadKeyPressed = useKeyPress(["r"]);
  let navHelpComboPressed = useKeyCombo(["Shift"], ["?"]);
  let clearTracesComboPressed = useKeyCombo(["Control"], ["l"]);

  // Navigate to previous trace
  useEffect(() => {
    if (prevTraceKeyPressed) {
      selectedIndex = selectedIndex > 0 ? selectedIndex - 1 : 0;
      summaryListRef.current?.scrollToItem(selectedIndex);

      selectedTraceID = traceIDs[selectedIndex];
      navigate(`/traces/${selectedTraceID}`);
    }
  }, [prevTraceKeyPressed]);

  // Navigate to next trace
  useEffect(() => {
    if (nextTraceKeyPressed) {
      selectedIndex =
        selectedIndex < traceIDs.length - 1
          ? selectedIndex + 1
          : traceIDs.length - 1;
      summaryListRef.current?.scrollToItem(selectedIndex);

      selectedTraceID = traceIDs[selectedIndex];
      navigate(`/traces/${selectedTraceID}`);
    }
  }, [nextTraceKeyPressed]);

  // Reload current window
  useEffect(() => {
    if (reloadKeyPressed) {
      window.location.reload();
    }
  }, [reloadKeyPressed]);

  // Show the keyboard navigation help modal
  useEffect(() => {
    if (navHelpComboPressed) {
      onOpen();
    }
  }, [navHelpComboPressed]);

  // Clear current traces
  useEffect(() => {
    if (clearTracesComboPressed) {
      clearTraceData();
    }
  }, [clearTracesComboPressed]);

  let itemData = {
    selectedTraceID: selectedTraceID,
    traceSummaries: traceSummaries,
  };

  let itemHeight = sidebarSummaryHeight + dividerHeight;

  return (
    <Flex
      ref={containerRef}
      height="100%"
      direction="column"
    >
      {/* Logs navigation button - feature flagged */}
      {enableLogs && (
        <Button
          as={NavLink}
          to="/logs"
          m={2}
          size="sm"
          colorScheme="blue"
          variant="outline"
        >
          View Logs
        </Button>
      )}
      
      {/* Metrics navigation button - feature flagged */}
      {enableMetrics && (
        <Button
          as={NavLink}
          to="/metrics"
          m={2}
          size="sm"
          colorScheme="green"
          variant="outline"
        >
          View Metrics
        </Button>
      )}
      <FixedSizeList
        height={size ? size.height - (enableLogs ? 50 : 0) - (enableMetrics ? 50 : 0) : 0}
        itemData={itemData}
        itemCount={traceSummaries.size}
        itemSize={itemHeight}
        width="100%"
        ref={summaryListRef}
      >
        {SidebarRow}
      </FixedSizeList>
      <KeyboardHelp
        isOpen={isOpen}
        onClose={onClose}
      />
    </Flex>
  );
}

export async function clearTraceData() {
  try {
    await telemetryAPI.clearTraces();
    window.location.replace("/");
  } catch (error) {
    throw new Error("Failed to clear traces: " + error);
  }
}