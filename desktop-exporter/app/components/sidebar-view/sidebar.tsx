import React from "react";
import { ArrowLeftIcon, ArrowRightIcon } from "@chakra-ui/icons";
import { Flex, IconButton, useColorModeValue } from "@chakra-ui/react";

import { TraceList } from "./trace-list";
import { TraceSummaryWithUIData } from "../../types/ui-types";

type SidebarProps = {
  isFullWidth: boolean;
  toggle: () => void;
  traceSummaries: TraceSummaryWithUIData[];
};

export function Sidebar(props: SidebarProps) {
  let sidebarColour = useColorModeValue("gray.50", "gray.700");
  let { isFullWidth, toggle, traceSummaries } = props;

  let sidebarWidth = "70px";
  let buttonIcon = <ArrowRightIcon />;
  let traceList = <></>;

  if (isFullWidth) {
    sidebarWidth = "350px";
    buttonIcon = <ArrowLeftIcon />;
    traceList = <TraceList traceSummaries={traceSummaries} />;
  }

  return (
    <Flex
      bgColor={sidebarColour}
      flexShrink="0"
      direction="column"
      transition="width 0.2s ease-in-out"
      width={sidebarWidth}
    >
      <Flex justifyContent="flex-end">
        <IconButton
          aria-label="Expand Sidebar"
          colorScheme="pink"
          icon={buttonIcon}
          margin="15px"
          onClick={toggle}
        />
      </Flex>
      {traceList}
    </Flex>
  );
}
