import React from "react";
import { Text, Flex, Spacer, useColorModeValue } from "@chakra-ui/react";

import { SpanWithMetadata } from "../../types/metadata-types";

type WaterfallRowData = {
  orderedSpans: SpanWithMetadata[];
  spanNameColumnWidth: number;
  serviceNameColumnWidth: number;
  selectedSpanID: string | undefined;
  setSelectedSpanID: (spanID: string) => void;
};

type WaterfallRowProps = {
  index: number;
  style: React.CSSProperties;
  data: WaterfallRowData;
};

export function WaterfallRow({ index, style, data }: WaterfallRowProps) {
  let {
    orderedSpans,
    spanNameColumnWidth,
    serviceNameColumnWidth,
    selectedSpanID,
    setSelectedSpanID,
  } = data;
  let { spanID, spanData } = orderedSpans[index];
  let spanDepth = orderedSpans[index].metadata.depth;

  // Set the background colour to make the list striped.
  let backgroundColour =
    index % 2 ? "" : useColorModeValue("gray.50", "gray.700");
  let selectedColour = useColorModeValue("pink.50", "pink.900");

  //Set the style for the selected item
  if (!!selectedSpanID && selectedSpanID === spanID) {
    backgroundColour = selectedColour;
  }

  // Set the padding to indicate parent/children relationship between spans
  let paddingLeft = spanDepth ? spanDepth * 25 : 0;

  // Add zero-width space after forward slashes, dashes, and dots
  // to indicate line breaking opportunity
  let nameLabel = spanData
    ? spanData.name
        .replaceAll("/", "/\u200B")
        .replaceAll("-", "-\u200B")
        .replaceAll(".", ".\u200B")
    : "missing span";

  let resourceLabel = spanData
    ? spanData.resource.attributes["service.name"]
    : "";
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
        paddingStart={2}
      >
        <Text
          noOfLines={2}
          fontSize="sm"
        >
          {nameLabel}
        </Text>
      </Flex>
      <Flex
        width={serviceNameColumnWidth}
        alignItems="center"
        paddingStart={3}
      >
        <Text fontSize="sm">{}</Text>
      </Flex>
      <Spacer />
    </Flex>
  );
}
