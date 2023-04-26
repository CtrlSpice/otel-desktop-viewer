import React from "react";
import {
  ArrowLeftIcon,
  ArrowRightIcon,
  DeleteIcon,
  MoonIcon,
  SunIcon,
} from "@chakra-ui/icons";
import {
  Flex,
  IconButton,
  useColorMode,
  useColorModeValue,
} from "@chakra-ui/react";

async function clearTraceData() {
  let response = await fetch("/api/clearData");
  if (!response.ok) {
    throw new Error("HTTP status " + response.status);
  } else {
    window.location.replace("/");
  }
}

type SidebarButtonsProps = {
  isFullWidth: boolean;
  isFullWidthDisabled: boolean;
  toggleSidebarWidth: () => void;
};

export function SidebarButtons(props: SidebarButtonsProps) {
  let { toggleColorMode } = useColorMode();
  let colourModeIcon = useColorModeValue(<MoonIcon />, <SunIcon />);
  let iconColour = useColorModeValue("white", "pink.900");
  let { isFullWidth, isFullWidthDisabled, toggleSidebarWidth } = props;

  if (isFullWidth) {
    return (
      <Flex
        height="100px"
        justifyContent="flex-end"
        alignItems="center"
      >
        <IconButton
          aria-label="Clear Trace Data"
          color={iconColour}
          colorScheme="pink"
          icon={<DeleteIcon />}
          marginEnd="16px"
          onClick={clearTraceData}
        />
        <IconButton
          aria-label="Toggle Colour Mode"
          color={iconColour}
          colorScheme="pink"
          icon={colourModeIcon}
          marginEnd="16px"
          onClick={toggleColorMode}
        />
        <IconButton
          aria-label="Collapse Sidebar"
          color={iconColour}
          colorScheme="pink"
          icon={<ArrowLeftIcon />}
          marginEnd="16px"
          onClick={toggleSidebarWidth}
        />
      </Flex>
    );
  }

  return (
    <>
      <IconButton
        aria-label="Expand Sidebar"
        color={iconColour}
        colorScheme="pink"
        icon={<ArrowRightIcon />}
        marginTop="16px"
        onClick={toggleSidebarWidth}
        isDisabled={isFullWidthDisabled}
      />
      <IconButton
        aria-label="Toggle Colour Mode"
        color={iconColour}
        colorScheme="pink"
        height="40px"
        icon={colourModeIcon}
        marginTop="16px"
        onClick={toggleColorMode}
        width="40px"
      />
    </>
  );
}
