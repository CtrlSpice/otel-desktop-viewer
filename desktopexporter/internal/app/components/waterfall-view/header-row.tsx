import React, { useRef } from "react";
import { useSize } from "@chakra-ui/react-use-size";
import { Flex, Heading, List, ListItem, Spacer, Text } from "@chakra-ui/react";
import { Duration } from "../../utils/duration";

type DurationIndicatorProps = {
  traceDuration: Duration;
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
  let { traceDuration } = props;
  let timeUnit = "ns";
  let totalMs = traceDuration.milliseconds;

  if (totalMs >= 1000) {
    timeUnit = "s";
  } else if (totalMs >= 1) {
    timeUnit = "ms";
  } else if (traceDuration.nanoseconds >= 1000) {
    timeUnit = "μs";
  }

  // Calculate section duration in milliseconds and nanoseconds
  let sectionMs = Math.floor(traceDuration.milliseconds / numSections);
  let sectionNs = Math.floor((traceDuration.milliseconds % numSections) * 1e6 + traceDuration.nanoseconds) / numSections;
  let sectionWidth = availableWidth / numSections;

  let durationSections = Array(numSections - 1)
    .fill(null)
    .map((_, i) => {
      // Calculate this section's time
      let sectionTimeMs = sectionMs * i;
      let sectionTimeNs = sectionNs * i;
      
      // Format the section time
      let sectionLabel = "";
      if (sectionTimeMs >= 1000) {
        sectionLabel = `${(sectionTimeMs / 1000).toFixed(3)} s`;
      } else if (sectionTimeMs >= 1) {
        sectionLabel = `${sectionTimeMs}.${sectionTimeNs.toString().padStart(6, '0')} ms`;
      } else if (sectionTimeNs >= 1000) {
        sectionLabel = `${(sectionTimeNs / 1000).toFixed(3)} μs`;
      } else {
        sectionLabel = `${sectionTimeNs} ns`;
      }

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
    <Text fontSize="x-small">{traceDuration.label}</Text>
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
  traceDuration: Duration;
};

export function HeaderRow(props: HeaderRowProps) {
  let {
    headerRowHeight,
    spanNameColumnWidth,
    serviceNameColumnWidth,
    traceDuration,
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
      <DurationIndicator traceDuration={traceDuration} />
    </Flex>
  );
}
