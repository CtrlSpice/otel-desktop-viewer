import React from "react";
import {
  ArrowLeftIcon,
  ArrowRightIcon,
  MoonIcon,
  SunIcon,
} from "@chakra-ui/icons";
import {
  Flex,
  IconButton,
  useColorMode,
  useColorModeValue,
} from "@chakra-ui/react";

import { TraceList } from "./trace-list";
import { TraceSummaryWithUIData } from "../../types/ui-types";

const sidebarFullWidth = 350;
const sidebarCollapsedWidth = 70;

type SidebarProps = {
  isFullWidth: boolean;
  toggle: () => void;
  traceSummaries: TraceSummaryWithUIData[];
};

export function Sidebar(props: SidebarProps) {
  let sidebarColour = useColorModeValue("gray.50", "gray.700");
  let iconColour = useColorModeValue("white", "pink.900");
  let { colorMode, toggleColorMode } = useColorMode();
  let { isFullWidth, toggle, traceSummaries } = props;

  if (isFullWidth) {
    return (
      <Flex
        backgroundColor={sidebarColour}
        flexShrink="0"
        direction="column"
        transition="width 0.2s ease-in-out"
        width={sidebarFullWidth}
      >
        <Flex
          height="100px"
          justifyContent="flex-end"
          alignItems="center"
        >
          <IconButton
            aria-label="Toddle Colour Mode"
            color={iconColour}
            colorScheme="pink"
            icon={colorMode === "light" ? <MoonIcon /> : <SunIcon />}
            marginEnd="16px"
            onClick={toggleColorMode}
          />
          <IconButton
            aria-label="Collapse Sidebar"
            color={iconColour}
            colorScheme="pink"
            icon={<ArrowLeftIcon />}
            marginEnd="16px"
            onClick={toggle}
          />
        </Flex>
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
      <IconButton
        aria-label="Expand Sidebar"
        color={iconColour}
        colorScheme="pink"
        icon={<ArrowRightIcon />}
        marginTop="16px"
        onClick={toggle}
      />
      <IconButton
        aria-label="Toddle Colour Mode"
        color={iconColour}
        colorScheme="pink"
        height="40px"
        icon={colorMode === "light" ? <MoonIcon /> : <SunIcon />}
        marginTop="16px"
        onClick={toggleColorMode}
        width="40px"
      />
    </Flex>
  );
}
