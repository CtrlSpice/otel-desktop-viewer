import React from "react";
import { Box, Flex, useColorModeValue } from "@chakra-ui/react";

import { getNsFromString, TraceTiming } from "../../utils/duration";
import { SpanData } from "../../types/api-types";

type DurationBarProps = {
  spanData: SpanData;
  traceTimeAttributes: TraceTiming;
  spanStartTimestamp: string;
  spanEndTimestamp: string;
};

export function DurationBar(props: DurationBarProps) {
  let durationBarColour = useColorModeValue("cyan.800", "cyan.700");

  let { traceStartTimeNS, traceDurationNS } = props.traceTimeAttributes;
  let spanStartTimeNs = getNsFromString(props.spanStartTimestamp);
  let spanEndTimeNs = getNsFromString(props.spanEndTimestamp);

  let offsetStart = Math.floor(
    ((spanStartTimeNs - traceStartTimeNS) / traceDurationNS) * 100,
  );
  let durationBarWidth = Math.round(
    ((spanEndTimeNs - spanStartTimeNs) / traceDurationNS) * 100,
  );
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
        position="relative"
        left={`${offsetStart}%`}
        width={`${durationBarWidth}%`}
        minWidth="5px"
      />
    </Flex>
  );
}
