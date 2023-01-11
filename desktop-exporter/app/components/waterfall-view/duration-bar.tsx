import React from "react";
import { Box, Flex, useColorModeValue } from "@chakra-ui/react";

import { NsFromString, TraceTimeAttributes } from "../../utils/duration";
import { EventData } from "../../types/api-types";

type DurationBarProps = {
  events: EventData[];
  traceTimeAttributes: TraceTimeAttributes;
  spanStartTimestamp: string;
  spanEndTimestamp: string;
};

export function DurationBar(props: DurationBarProps) {
  let durationBarColour = useColorModeValue("cyan.800", "cyan.700");

  let { traceStartTimeNS, traceDurationNS } = props.traceTimeAttributes;
  let spanStartTimeNs = NsFromString(props.spanStartTimestamp);
  let spanEndTimeNs = NsFromString(props.spanEndTimestamp);

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
