import React from "react";
import {
  Button,
  Flex,
  Heading,
  Spacer,
  useColorMode,
  useColorModeValue,
} from "@chakra-ui/react";
import { SunIcon, MoonIcon } from "@chakra-ui/icons";

export function Header(props) {
  const { colorMode, toggleColorMode } = useColorMode();
  return (
    <Flex
      className="header"
      bg={useColorModeValue("gray.100", "gray.900")}
      align="center"
      px={2}
    >
      <Heading
        as="h1"
        size="md"
        noOfLines={1}
      >
        Trace ID: {props.traceID}
      </Heading>

      <Spacer />

      <Button
        aria-label="Toggle Colour Mode"
        onClick={toggleColorMode}
        w="fit-content"
      >
        {colorMode === "light" ? <MoonIcon /> : <SunIcon />}
      </Button>
    </Flex>
  );
}
