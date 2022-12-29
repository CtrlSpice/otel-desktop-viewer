import React from "react";
import { Text, Flex, Spacer, useColorModeValue } from "@chakra-ui/react";

import { SpanData } from "../../types/api-types";

type WaterfallRowData = {
  spans: SpanData[];
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
    spans,
    spanNameColumnWidth,
    serviceNameColumnWidth,
    selectedSpanID,
    setSelectedSpanID,
  } = data;
  let span = spans[index];

  // Set the background colour to make the list striped.
  let backgroundColour =
    index % 2 ? "" : useColorModeValue("gray.50", "gray.700");
  let selectedColour = useColorModeValue("pink.50", "pink.900");

  //Set the style for the selected item
  if (!!selectedSpanID && selectedSpanID === span.spanID) {
    backgroundColour = selectedColour;
  }

  // Add zero-width space after forward slashes, dashes, and dots
  // to indicate line breaking opportunity
  let nameLabel = span.name
    .replaceAll("/", "/\u200B")
    .replaceAll("-", "-\u200B")
    .replaceAll(".", ".\u200B");

  return (
    <Flex
      style={style}
      bgColor={backgroundColour}
      onClick={() => setSelectedSpanID(span.spanID)}
    >
      <Flex
        width={spanNameColumnWidth}
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
        <Text fontSize="sm">{span.resource.attributes["service.name"]}</Text>
      </Flex>
      <Spacer />
    </Flex>
  );
}
