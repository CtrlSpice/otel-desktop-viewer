import React, { useRef, ClipboardEvent } from "react";
import { FixedSizeList } from "react-window";
import {
  Text,
  Flex,
  Heading,
  Spacer,
  useColorModeValue,
} from "@chakra-ui/react";
import { useSize } from "@chakra-ui/react-use-size";

import { SpanData } from "../../types/api-types";

const waterfallItemHeight = 50;
const headerRowHeight = 30;
const spanNameColumnWidth = 244;
const serviceNameColumnWidth = 120;

type WaterfallRowProps = {
  index: number;
  style: React.CSSProperties;
  data: WaterfallViewProps;
};

function WaterfallRow({ index, style, data }: WaterfallRowProps) {
  let { spans, selectedSpanID, setSelectedSpanID } = data;
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

  let stripZeroWidthSpaces = (e: ClipboardEvent<HTMLParagraphElement>) => {
    let selection = window.getSelection();
    if (!selection) {
      return;
    }
    let text = selection.toString().replaceAll("\u200B", "");
    e.clipboardData?.setData("text/plain", text);
    e.preventDefault();
  };

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
          onCopy={(e) => stripZeroWidthSpaces(e)}
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

function HeaderRow() {
  return (
    <Flex height={`${headerRowHeight}px`}>
      <Flex
        width={spanNameColumnWidth}
        alignItems="center"
      >
        <Heading
          paddingStart={2}
          size="sm"
        >
          name
        </Heading>
      </Flex>
      <Flex
        width={serviceNameColumnWidth}
        alignItems="center"
        paddingStart={3}
      >
        <Heading size="sm">service.name</Heading>
      </Flex>
      <Spacer />
    </Flex>
  );
}

type WaterfallViewProps = {
  spans: SpanData[];
  selectedSpanID: string | undefined;
  setSelectedSpanID: (spanID: string) => void;
};

export function WaterfallView(props: WaterfallViewProps) {
  const ref = useRef(null);
  const size = useSize(ref);

  return (
    <Flex
      direction="column"
      ref={ref}
      height="100%"
    >
      <HeaderRow />
      <FixedSizeList
        className="List"
        height={size ? size.height - headerRowHeight : 0}
        itemData={props}
        itemCount={props.spans.length}
        itemSize={waterfallItemHeight}
        width={"100%"}
      >
        {WaterfallRow}
      </FixedSizeList>
    </Flex>
  );
}
