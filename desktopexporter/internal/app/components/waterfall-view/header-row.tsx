import React, { useRef } from "react";
import { useSize } from "@chakra-ui/react-use-size";
import { Flex, Heading, List, ListItem, Spacer, Text } from "@chakra-ui/react";
import { PreciseTimestamp } from "../../types/precise-timestamp";
import { formatDuration } from "../../utils/duration";

type DurationIndicatorProps = {
  traceBounds: { startTime: PreciseTimestamp; endTime: PreciseTimestamp };
};

function DurationIndicator(props: DurationIndicatorProps) {
  let ref = useRef(null);
  let size = useSize(ref);

  // Determine the number of sections our bar will have
  let availableWidth = size ? size.width : 0;
  let numSections = 1;
  if (availableWidth >= 800) {
    numSections = 8;
  } else if (availableWidth >= 600) {
    numSections = 6;
  } else if (availableWidth >= 300) {
    numSections = 4;
  } else if (availableWidth >= 100) {
    numSections = 2;
  }

  // Determine the time unit we are working in
  let { traceBounds } = props;
  let duration = traceBounds.endTime.nanoseconds - traceBounds.startTime.nanoseconds;
  let sectionNs = duration / BigInt(numSections);
  let sectionWidth = availableWidth / numSections;

  let durationSections = Array(numSections - 1)
    .fill(null)
    .map((_, i) => {
      let sectionLabel = formatDuration(sectionNs * BigInt(i));

      return (
        <ListItem
          key={i}
          float="left"
        >
          <Text
            fontSize="x-small"
            width={sectionWidth}
          >
            {sectionLabel}
          </Text>
        </ListItem>
      );
    });

  let lastDurationLabel = (
    <Text fontSize="x-small">{formatDuration(duration)}</Text>
  );

  return (
    <Flex
      alignItems="center"
      height="100%"
      flex-direction="row"
      flex="1 1 auto"
      marginX={2}
      ref={ref}
    >
      <List>{durationSections}</List>
      <Spacer />
      {lastDurationLabel}
    </Flex>
  );
}

type HeaderRowProps = {
  headerRowHeight: number;
  spanNameColumnWidth: number;
  serviceNameColumnWidth: number;
  traceBounds: { startTime: PreciseTimestamp; endTime: PreciseTimestamp };
};

export function HeaderRow(props: HeaderRowProps) {
  let {
    headerRowHeight,
    spanNameColumnWidth,
    serviceNameColumnWidth,
    traceBounds,
  } = props;

  return (
    <Flex height={`${headerRowHeight}px`}>
      <Flex
        width={spanNameColumnWidth}
        alignItems="center"
      >
        <Heading
          paddingX={2}
          size="sm"
        >
          name
        </Heading>
      </Flex>
      <Flex
        width={serviceNameColumnWidth}
        alignItems="center"
      >
        <Heading
          paddingX={1}
          size="sm"
        >
          service.name
        </Heading>
      </Flex>
      <DurationIndicator traceBounds={traceBounds} />
    </Flex>
  );
}
