import React from "react";
import {
  Button,
  Flex,
  Heading,
  IconButton,
  Spacer,
  useColorMode,
  useColorModeValue,
} from "@chakra-ui/react";
import { SunIcon, MoonIcon } from "@chakra-ui/icons";

type HeaderProps = {
  traceID: string;
};

export function Header(props: HeaderProps) {
  const { colorMode, toggleColorMode } = useColorMode();
  return (
    <Flex
      bg={useColorModeValue("gray.100", "gray.900")}
      align="center"
      height={"60px"}
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

      <IconButton
        aria-label="Toddle Colour Mode"
        colorScheme="pink"
        icon={colorMode === "light" ? <MoonIcon /> : <SunIcon />}
        margin="15px"
        onClick={toggleColorMode}
      />
    </Flex>
  );
}
