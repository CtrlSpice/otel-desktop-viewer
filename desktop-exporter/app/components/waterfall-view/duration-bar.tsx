import React, { useRef } from "react";
import {
  Box,
  Circle,
  Flex,
  List,
  Text,
  Tooltip,
  useColorModeValue,
} from "@chakra-ui/react";
import { useSize } from "@chakra-ui/react-use-size";

import {
  getDurationString,
  getNsFromString,
  TraceTiming,
} from "../../utils/duration";
import { EventData, SpanData } from "../../types/api-types";

type EventDotsListProps = {
  events: EventData[];
  spanStartTimeNs: number;
  spanEndTimeNs: number;
};

function EventDotsList(props: EventDotsListProps) {
  let { events, spanStartTimeNs, spanEndTimeNs } = props;

  let eventDotsList = events.map((eventData) => {
    let eventName = eventData.name;
    let eventTimeNs = getNsFromString(eventData.timestamp);
    let spanDurationNS = spanEndTimeNs - spanStartTimeNs;
    let eventOffsetPercent = Math.floor(
      ((eventTimeNs - spanStartTimeNs) / spanDurationNS) * 100,
    );

    return (
      <li key={`${eventName}-${eventData.timestamp}`}>
        <Tooltip
          hasArrow
          label={eventName}
          placement="top"
        >
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
        </Tooltip>
      </li>
    );
  });

  return <List>{eventDotsList}</List>;
}

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

  return (
    <Flex
      border="0"
      marginX={2}
      marginY="16px"
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
        <Flex
          position="absolute"
          width={`${labelWidth}px`}
          left={labelOffset}
          justifyContent="center"
        >
          <Text
            fontSize="xs"
            fontWeight="700"
            paddingLeft={2}
            color={labelTextColour}
          >
            {label}
          </Text>
        </Flex>
        <EventDotsList
          events={props.spanData.events}
          spanStartTimeNs={spanStartTimeNs}
          spanEndTimeNs={spanEndTimeNs}
        />
      </Box>
    </Flex>
  );
}
