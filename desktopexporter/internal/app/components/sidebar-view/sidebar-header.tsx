import React from "react";
import {
  ArrowLeftIcon,
  ArrowRightIcon,
  DeleteIcon,
  MoonIcon,
  RepeatIcon,
  SunIcon,
} from "@chakra-ui/icons";
import {
  Button,
  Flex,
  IconButton,
  Spacer,
  Text,
  useColorMode,
  useColorModeValue,
} from "@chakra-ui/react";
import { clearTraceData } from "./trace-list";

type SidebarHeaderProps = {
  isFullWidth: boolean;
  isFullWidthDisabled: boolean;
  toggleSidebarWidth: () => void;
  numNewTraces: number;
};

export function SidebarHeader(props: SidebarHeaderProps) {
  let { toggleColorMode } = useColorMode();
  let colourModeIcon = useColorModeValue(<MoonIcon />, <SunIcon />);
  let { isFullWidth, isFullWidthDisabled, toggleSidebarWidth, numNewTraces } =
    props;

  if (isFullWidth) {
    return (
      <Flex
        direction="column"
        height="fit-content"
        justifyContent="space-evenly"
      >
        <Flex
          justifyContent="flex-start"
          alignItems="center"
          height="50px"
        >
          <Button
            size="md"
            aria-label="Clear Trace Data"
            variant="ghost"
            colorScheme="pink"
            fontWeight="normal"
            leftIcon={<DeleteIcon />}
            marginStart="10px"
            onClick={clearTraceData}
          >
            <Text
              fontSize="sm"
              fontWeight="bold"
              color="ButtonText"
            >
              Clear Traces
            </Text>
          </Button>
          <Spacer />
          <IconButton
            size="md"
            aria-label="Toggle Colour Mode"
            variant="ghost"
            colorScheme="pink"
            icon={colourModeIcon}
            marginEnd="2px"
            onClick={toggleColorMode}
          />
          <IconButton
            size="md"
            aria-label="Collapse Sidebar"
            variant="ghost"
            colorScheme="pink"
            icon={<ArrowLeftIcon />}
            marginEnd="10px"
            onClick={toggleSidebarWidth}
          />
        </Flex>
        <Flex
          justifyContent="flex-start"
          alignItems="center"
          transition="height 0.2s ease-in-out"
          height={numNewTraces > 0 ? "50px" : 0}
          overflow="hidden"
        >
          <Button
            size="md"
            aria-label="Refresh"
            variant="ghost"
            colorScheme="pink"
            fontWeight="normal"
            leftIcon={<RepeatIcon />}
            marginX="10px"
            justifyContent="flex-start"
            onClick={() => {
              window.location.reload();
            }}
          >
            <Text
              fontSize="sm"
              fontWeight="bold"
              color="ButtonText"
            >
              {numNewTraces} New Trace{numNewTraces === 1 ? " " : "s"}
            </Text>
          </Button>
        </Flex>
      </Flex>
    );
  }

  return (
    <>
      <IconButton
        size="md"
        aria-label="Expand Sidebar"
        colorScheme="pink"
        variant="ghost"
        icon={<ArrowRightIcon />}
        marginTop="10px"
        onClick={toggleSidebarWidth}
        isDisabled={isFullWidthDisabled}
      />
      <IconButton
        size="md"
        aria-label="Toggle Colour Mode"
        colorScheme="pink"
        variant="ghost"
        icon={colourModeIcon}
        marginTop="2px"
        onClick={toggleColorMode}
      />
    </>
  );
}
