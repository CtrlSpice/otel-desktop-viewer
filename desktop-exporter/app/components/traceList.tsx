import React, { useRef } from "react";
import { FixedSizeList } from "react-window";
import { NavLink, useMatch } from "react-router-dom";
import {
  Flex,
  Heading,
  LinkBox,
  LinkOverlay,
  Text,
  useColorModeValue,
} from "@chakra-ui/react";
import { useSize } from "@chakra-ui/react-use-size";

import { TraceSummary } from "../types/api-types";

const listItemHeight = 100;
const listItemBottomMargin = 2;

type RowProps = {
  index: number;
  style: Object;
  data: TraceSummary[];
};

function Row({ index, style, data }: RowProps) {
  let isSelected = useMatch(`traces/${data[index].traceID}`);
  let backgroundColour = isSelected
    ? useColorModeValue("whiteAlpha.800", "whiteAlpha.400")
    : useColorModeValue("whiteAlpha.600", "whiteAlpha.200");
  let borderLeft = isSelected ? "7px solid" : "none";

  return (
    <div style={style}>
      <LinkBox
        bgColor={backgroundColour}
        height="100px"
        padding="10px"
        marginBottom={`${listItemBottomMargin}px`}
        borderLeft={borderLeft}
        borderColor="pink.500"
      >
        <Heading
          marginY="1"
          noOfLines={1}
          size="sm"
        >
          <LinkOverlay
            as={NavLink}
            to={`traces/${data[index].traceID}`}
          >
            {data[index].traceID}
          </LinkOverlay>
        </Heading>
        <Text>
          <strong>Number of spans: </strong>
          {data[index].spanCount}
        </Text>
        <Text>
          <strong>Duration: </strong>
          {data[index].durationMS} ms
        </Text>
      </LinkBox>
    </div>
  );
}

type TraceListProps = {
  traceSummaries: TraceSummary[];
};

export function TraceList(props: TraceListProps) {
  const ref = useRef(null);
  const size = useSize(ref);

  return (
    <Flex
      ref={ref}
      height="100%"
    >
      <FixedSizeList
        height={size ? size.height : 0}
        itemData={props.traceSummaries}
        itemCount={props.traceSummaries.length}
        itemSize={listItemHeight + listItemBottomMargin}
        width="100%"
      >
        {Row}
      </FixedSizeList>
    </Flex>
  );
}