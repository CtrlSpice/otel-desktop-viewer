import React from "react";
import { ArrowLeftIcon, ArrowRightIcon } from "@chakra-ui/icons";
import { Flex, IconButton, useColorModeValue } from "@chakra-ui/react";

import { TraceList } from "../components/traceList";
import { TraceSummary } from "../types/api-types";

type SidebarProps = {
  isFullWidth: boolean;
  toggle: () => void;
  traceSummaries: TraceSummary[];
};

export function Sidebar(props: SidebarProps) {
  const sidebarColour = useColorModeValue("pink.100", "pink.900");

  let sidebarWidth = "70px";
  let buttonIcon = <ArrowRightIcon />;
  let traceList = <></>;

  if (props.isFullWidth) {
    sidebarWidth = "250px";
    buttonIcon = <ArrowLeftIcon />;
    traceList = <TraceList traceSummaries={props.traceSummaries} />;
  }

  return (
    <Flex
      bgColor={sidebarColour}
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
          onClick={props.toggle}
        />
      </Flex>
      {traceList}
    </Flex>
  );
}
