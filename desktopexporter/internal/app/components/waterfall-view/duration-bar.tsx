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

import { formatDuration, getOffset } from "../../utils/duration";
import { EventData, SpanData } from "../../types/api-types";
import { PreciseTimestamp } from "../../types/precise-timestamp";

type EventDotsListProps = {
  events: EventData[];
  spanStartTime: PreciseTimestamp;
  spanEndTime: PreciseTimestamp;
};

function EventDotsList(props: EventDotsListProps) {
  let { events, spanStartTime, spanEndTime } = props;

  let eventDotsList = events.map((eventData) => {
    let eventName = eventData.name;
    let eventOffsetPercent = getOffset(
      spanStartTime,
      spanEndTime,
      eventData.timestamp
    );

    return (
      <li key={`${eventName}-${eventData.timestamp.toString()}`}>
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
  traceBounds: { startTime: PreciseTimestamp; endTime: PreciseTimestamp };
  spanData: SpanData;
};

export function DurationBar(props: DurationBarProps) {
  const ref = useRef(null);
  const size = useSize(ref);

  // approximate width of the label in pixels
  const labelWidth = 80;

  let durationBarColour = useColorModeValue("cyan.800", "cyan.700");
  let labelTextColour = useColorModeValue("blackAlpha.800", "white");

  let { traceBounds } = props;
  let { startTime, endTime } = props.spanData;
  let barOffsetPercent = getOffset(traceBounds.startTime, traceBounds.endTime, startTime);
  let barEndPercent = getOffset(traceBounds.startTime, traceBounds.endTime, endTime);
  let barWidthPercent = barEndPercent - barOffsetPercent;

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
            whiteSpace="nowrap"
          >
            {formatDuration(endTime.nanoseconds - startTime.nanoseconds)}
          </Text>
        </Flex>
        <EventDotsList
          events={props.spanData.events}
          spanStartTime={startTime}
          spanEndTime={endTime}
        />
      </Box>
    </Flex>
  );
}
