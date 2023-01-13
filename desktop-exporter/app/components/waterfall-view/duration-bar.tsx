import React, { useRef } from "react";
import {
  Box,
  Circle,
  Flex,
  LightMode,
  List,
  Text,
  useColorModeValue,
} from "@chakra-ui/react";
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

  let barOffsetPercent = Math.floor(
    ((spanStartTimeNs - traceStartTimeNS) / traceDurationNS) * 100,
  );
  let barWidthPercent = Math.round(
    ((spanEndTimeNs - spanStartTimeNs) / traceDurationNS) * 100,
  );

  let labelOffset;
  if (size && size.width >= labelWidth) {
    // Label is inside the bar
    labelOffset = "0px";
    labelTextColour = "white";
  } else if (size && barOffsetPercent < 50) {
    // Label is left of the bar
    labelOffset = `${Math.floor(size.width)}px`;
  } else {
    // Label is right of the bar
    labelOffset = `${Math.floor(-labelWidth)}px`;
  }

  let label = getDurationString(spanEndTimeNs - spanStartTimeNs);

  let eventCircles = props.spanData.events.map((eventData, index) => {
    let eventTimeNs = getNsFromString(eventData.timestamp);
    let spanDurationNS = spanEndTimeNs - spanStartTimeNs;
    let eventOffsetPercent = Math.floor(
      ((eventTimeNs - spanStartTimeNs) / spanDurationNS) * 100,
    );

    return (
      <li key={index}>
        <Circle
          size="18px"
          bg="whiteAlpha.400"
          border="solid 1px"
          borderColor="cyan.800"
          position="absolute"
          left={`${eventOffsetPercent}%`}
          transformOrigin="center"
          transform="translate(-50%)"
        />
      </li>
    );
  });
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
        left={`${barOffsetPercent}%`}
        width={`${barWidthPercent}%`}
        minWidth="2px"
        ref={ref}
      >
        <List>{eventCircles}</List>
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
