import React, { useRef } from "react";
import { Box, Flex, Text, useColorModeValue } from "@chakra-ui/react";
import { useSize } from "@chakra-ui/react-use-size";

import {
  getDurationString,
  getNsFromString,
  TraceTiming,
} from "../../utils/duration";
import { SpanData } from "../../types/api-types";

type DurationBarProps = {
  spanData: SpanData;
  traceTimeAttributes: TraceTiming;
  spanStartTimestamp: string;
  spanEndTimestamp: string;
};

export function DurationBar(props: DurationBarProps) {
  const ref = useRef(null);
  const size = useSize(ref);
  // approximate width of the label in pixels
  const labelWidth = 80;

  let durationBarColour = useColorModeValue("cyan.800", "cyan.700");
  let labelTextColour = useColorModeValue("blackAlpha.800", "white");

  let { traceStartTimeNS, traceDurationNS } = props.traceTimeAttributes;
  let spanStartTimeNs = getNsFromString(props.spanStartTimestamp);
  let spanEndTimeNs = getNsFromString(props.spanEndTimestamp);

  let barOffset = Math.floor(
    ((spanStartTimeNs - traceStartTimeNS) / traceDurationNS) * 100,
  );
  let barWidth = Math.round(
    ((spanEndTimeNs - spanStartTimeNs) / traceDurationNS) * 100,
  );

  let labelOffset;
  if (size && size.width >= labelWidth) {
    labelOffset = "0px";
    labelTextColour = "white";
  } else if (size && barOffset < 50) {
    labelOffset = `${Math.floor(size.width)}px`;
  } else {
    labelOffset = `${Math.floor(-labelWidth)}px`;
  }

  let label = getDurationString(spanEndTimeNs - spanStartTimeNs);
  return (
    <Flex
      border="0"
      marginX={2}
      marginY="16px"
      overflow="hidden"
      width="100%"
    >
      <Box
        bgColor={durationBarColour}
        borderRadius="md"
        overflow="visible"
        position="relative"
        left={`${barOffset}%`}
        width={`${barWidth}%`}
        minWidth="2px"
        ref={ref}
      >
        <Text
          fontSize="xs"
          fontWeight="700"
          paddingLeft={2}
          color={labelTextColour}
          position="absolute"
          width={`${labelWidth}px`}
          left={labelOffset}
        >
          {label}
        </Text>
      </Box>
    </Flex>
  );
}
