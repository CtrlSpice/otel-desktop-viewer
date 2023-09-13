import React from "react";
import { Text, Flex, Spacer, useColorModeValue,IconButton } from "@chakra-ui/react";
import { ChevronDownIcon,ChevronRightIcon,MinusIcon, WarningTwoIcon } from "@chakra-ui/icons";
import { SpanDataStatus, SpanWithUIData } from "../../types/ui-types";
import { TraceTiming } from "../../utils/duration";
import { DurationBar } from "./duration-bar";

type WaterfallRowData = {
  orderedSpans: SpanWithUIData[];
  traceTimeAttributes: TraceTiming;
  spanNameColumnWidth: number;
  serviceNameColumnWidth: number;
  selectedSpanID: string | undefined;
  setSelectedSpanID: (spanID: string) => void;
  toggle: (id: string) => void;
};

type WaterfallRowProps = {
  index: number;
  style: React.CSSProperties;
  data: WaterfallRowData;
};

export function WaterfallRow({ index, style, data }: WaterfallRowProps) {
  let selectedColour = useColorModeValue("pink.100", "pink.900");
  let oddStripeColour = useColorModeValue("gray.50", "gray.700");
  // Set the background colour to make the list striped.
  let backgroundColour = index % 2 ? "" : oddStripeColour;

  let {
    orderedSpans,
    traceTimeAttributes,
    spanNameColumnWidth,
    serviceNameColumnWidth,
    selectedSpanID,
    setSelectedSpanID,
    toggle,
  } = data;

  let span = orderedSpans[index];
  let { spanID, depth } = span.metadata;

  if (span.status === SpanDataStatus.present) {
    let { spanData } = span;

    // Set the margin to indicate parent/children relationship between spans
    let paddingLeft = depth * 25;

    //Set the style for the selected item
    if (!!selectedSpanID && selectedSpanID === spanID) {
      backgroundColour = selectedColour;
    }
    // Add zero-width space after forward slashes, dashes, and dots
    // to indicate line breaking opportunity
    let nameLabel = spanData.name
      .replaceAll("/", "/\u200B")
      .replaceAll("-", "-\u200B")
      .replaceAll(".", ".\u200B");

    let resourceLabel = spanData.resource.attributes["service.name"];

    let icon = <ChevronDownIcon />
    if (span.metadata.toggled) {
      icon = <ChevronRightIcon />
    }
    
    return (
      <Flex
        style={style}
        bgColor={backgroundColour}
        paddingLeft={`${paddingLeft}px`}
        onClick={() => setSelectedSpanID(spanID)}
      > 
        <Flex
          width={spanNameColumnWidth - paddingLeft}
          alignItems="center"
          flexGrow="1"
          flexShrink="0"
        >
          <IconButton
            size="md"
            aria-label="Collapse Sidebar"
            variant="ghost"
            colorScheme="pink"
            icon={icon}
            marginEnd="10px"
            onClick={() => toggle(spanID)}
          /> 
          <Text
            paddingX={2}
            noOfLines={2}
            fontSize="sm"
          >
            {nameLabel}
          </Text>
        </Flex>
        <Flex
          width={serviceNameColumnWidth}
          alignItems="center"
          flexGrow="1"
          flexShrink="0"
        >
          <Text
            paddingX={2}
            fontSize="sm"
          >
            {resourceLabel}
          </Text>
        </Flex>
        <DurationBar
          spanData={spanData}
          traceTimeAttributes={traceTimeAttributes}
          spanStartTimestamp={spanData.startTime}
          spanEndTimestamp={spanData.endTime}
        />
      </Flex>
    );
  }
  return (
    <Flex
      style={style}
      alignItems="center"
      bgColor={backgroundColour}
      paddingStart={2}
      experimental_spaceX={2}
    >
      <WarningTwoIcon color="orange.500" />
      <Text fontSize="sm">{`Missing Span [Span ID:${spanID}]`}</Text>
    </Flex>
  );
}
