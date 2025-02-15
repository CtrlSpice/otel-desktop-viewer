import React from "react";
import { Flex, useColorModeValue } from "@chakra-ui/react";

import { TraceList } from "./trace-list";
import { TraceSummaryWithUIData } from "../../types/ui-types";
import { SidebarHeader } from "./sidebar-header";

const sidebarFullWidth = 350;
const sidebarCollapsedWidth = 70;

type SidebarProps = {
  isFullWidth: boolean;
  toggleSidebarWidth: () => void;
  traceSummaries: Map<string, TraceSummaryWithUIData>;
  numNewTraces: number;
};

export function Sidebar(props: SidebarProps) {
  let sidebarColour = useColorModeValue("gray.50", "gray.700");
  let { isFullWidth, toggleSidebarWidth, traceSummaries, numNewTraces } = props;
  let isFullWidthDisabled = traceSummaries.size === 0;

  if (isFullWidth) {
    return (
      <Flex
        backgroundColor={sidebarColour}
        flexShrink="0"
        direction="column"
        transition="width 0.2s ease-in-out"
        width={sidebarFullWidth}
      >
        <SidebarHeader
          isFullWidth={isFullWidth}
          toggleSidebarWidth={toggleSidebarWidth}
          isFullWidthDisabled={false}
          numNewTraces={numNewTraces}
        />
        <TraceList traceSummaries={traceSummaries} />
      </Flex>
    );
  }

  return (
    <Flex
      alignItems="center"
      backgroundColor={sidebarColour}
      flexShrink="0"
      direction="column"
      transition="width 0.2s ease-in-out"
      width={sidebarCollapsedWidth}
    >
      <SidebarHeader
        isFullWidth={isFullWidth}
        isFullWidthDisabled={isFullWidthDisabled}
        toggleSidebarWidth={toggleSidebarWidth}
        numNewTraces={0}
      />
    </Flex>
  );
}
