import React, { useRef } from "react";
import { useSize } from "@chakra-ui/react-use-size";
import { Flex, Heading, Spacer, Text } from "@chakra-ui/react";

type DurationIndicatorProps = {
  traceDuration: number;
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
  if (traceDuration >= 1e9) {
    traceDuration = traceDuration / 1e9;
    timeUnit = "s";
  } else if (traceDuration >= 1e6) {
    traceDuration = traceDuration / 1e6;
    timeUnit = "ms";
  } else if (traceDuration >= 1e3) {
    traceDuration = traceDuration / 1e3;
    timeUnit = "μs";
  }

  let sectionDuration = traceDuration / numSections;
  let sectionWidth = availableWidth / numSections;

  let durationSections = Array(numSections - 1)
    .fill(null)
    .map((_, i) => {
      let sectionLabel = `${+(sectionDuration * i).toFixed(3)}${timeUnit}`;
      return (
        <Text
          key={i}
          fontSize="x-small"
          width={sectionWidth}
        >
          {sectionLabel}
        </Text>
      );
    });
  durationSections.push(<Spacer />);
  durationSections.push(
    <Text
      key={numSections - 1}
      fontSize="x-small"
    >
      {`${+traceDuration.toFixed(3)}${timeUnit}`}
    </Text>,
  );

  return (
    <Flex
      alignItems="center"
      height="100%"
      flex-direction="row"
      flex="1 1 auto"
      ref={ref}
    >
      {durationSections}
    </Flex>
  );
}

type HeaderRowProps = {
  headerRowHeight: number;
  spanNameColumnWidth: number;
  serviceNameColumnWidth: number;
  traceDuration: number;
};

export function HeaderRow(props: HeaderRowProps) {
  let {
    headerRowHeight,
    spanNameColumnWidth,
    serviceNameColumnWidth,
    traceDuration,
  } = props;

  return (
    <Flex
      height={`${headerRowHeight}px`}
      experimental_spaceX={2}
    >
      <Flex
        width={spanNameColumnWidth}
        alignItems="center"
      >
        <Heading size="sm">name</Heading>
      </Flex>
      <Flex
        width={serviceNameColumnWidth}
        alignItems="center"
      >
        <Heading size="sm">service.name</Heading>
      </Flex>
      <DurationIndicator traceDuration={traceDuration} />
    </Flex>
  );
}
