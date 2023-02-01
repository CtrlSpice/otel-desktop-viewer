import React from "react";
import { Flex, useColorModeValue } from "@chakra-ui/react";

import { TraceList } from "./trace-list";
import { TraceSummaryWithUIData } from "../../types/ui-types";
import { SidebarButtons } from "./sidebar-buttons";

const sidebarFullWidth = 350;
const sidebarCollapsedWidth = 70;

type SidebarProps = {
  isFullWidth: boolean;
  toggleSidebarWidth: () => void;
  traceSummaries: TraceSummaryWithUIData[];
};

export function Sidebar(props: SidebarProps) {
  let sidebarColour = useColorModeValue("gray.50", "gray.700");
  let { isFullWidth, toggleSidebarWidth, traceSummaries } = props;
  let isFullWidthDisabled = traceSummaries.length === 0;

  if (isFullWidth) {
    return (
      <Flex
        backgroundColor={sidebarColour}
        flexShrink="0"
        direction="column"
        transition="width 0.2s ease-in-out"
        width={sidebarFullWidth}
      >
        <SidebarButtons
          isFullWidth={isFullWidth}
          toggleSidebarWidth={toggleSidebarWidth}
          isFullWidthDisabled={false}
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
      <SidebarButtons
        isFullWidth={isFullWidth}
        isFullWidthDisabled={isFullWidthDisabled}
        toggleSidebarWidth={toggleSidebarWidth}
      />
    </Flex>
  );
}
