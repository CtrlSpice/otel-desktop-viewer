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

type RowProps = {
  index: number;
  style: Object;
  data: TraceSummary[];
};

function Row({ index, style, data }: RowProps) {
  let isSelected = useMatch(`traces/${data[index].traceID}`);

  let backgroundColour = isSelected
    ? useColorModeValue("teal.100", "teal.700")
    : useColorModeValue("whiteAlpha.700", "whiteAlpha.400");

  return (
    <div style={style}>
      <LinkBox
        bgColor={backgroundColour}
        height="100px"
        padding="10px"
        marginX="10px"
        marginTop="10px"
        rounded="md"
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
        <Text>Number of spans: {data[index].spanCount}</Text>
        <Text>Duration: {data[index].durationMS} ms</Text>
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
        itemSize={110}
        width="100%"
      >
        {Row}
      </FixedSizeList>
    </Flex>
  );
}