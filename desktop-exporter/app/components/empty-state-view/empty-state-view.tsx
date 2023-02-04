import React, { useEffect, useState } from "react";
import {
  Alert,
  AlertIcon,
  AlertTitle,
  Box,
  Button,
  Card,
  CardBody,
  CardFooter,
  CardHeader,
  Flex,
  Heading,
  Image,
  ListItem,
  OrderedList,
  Stack,
  Text,
  useBoolean,
  useColorModeValue,
  useInterval,
} from "@chakra-ui/react";

async function loadSampleData() {
  let response = await fetch("/api/sampleData");
  if (!response.ok) {
    throw new Error("HTTP status " + response.status);
  } else {
    window.location.reload();
  }
}

function SampleDataButton() {
  let [isLoading, setIsLoading] = useBoolean(false);

  if (isLoading) {
    return (
      <Button
        isLoading
        colorScheme="pink"
        loadingText="Loading"
        spinnerPlacement="start"
      />
    );
  }

  return (
    <Button
      colorScheme="pink"
      onClick={() => {
        setIsLoading.on();
        loadSampleData();
      }}
    >
      Load Sample Data
    </Button>
  );
}

async function pollTraceCount() {
  let response = await fetch("/api/traceCount");
  if (!response.ok) {
    throw new Error("HTTP status " + response.status);
  } else {
    let { numTraces } = await response.json();
    if (numTraces > 0) {
      window.location.reload();
    }
  }
}

export function EmptyStateView() {
  let alertColour = useColorModeValue("cyan.700", "cyan.300");
  useInterval(pollTraceCount, 500);

  return (
    <Flex
      flexDirection="column"
      align="center"
      width="100%"
      overflowY="scroll"
    >
      <Alert
        status="info"
        variant="solid"
        minHeight="64px"
        backgroundColor={alertColour}
      >
        <AlertIcon boxSize="24px" />
        <AlertTitle fontSize="md">
          {"Nothing here yet. Waiting for data..."}
        </AlertTitle>
      </Alert>
      <Card
        align="center"
        width="50%"
        maxWidth="700px"
        margin="64px"
        variant="filled"
      >
        <CardHeader>
          <Image
            src="assets/images/lulu.jpg"
            alt="A pink axolotl is striking a heroic pose while gazing at a field of stars through a telescope. Her name is Lulu Axol'Otel the First, valiant adventurer and observability queen."
            borderRadius="lg"
          />
          <Text size="sm">Artwork credit goes here</Text>
        </CardHeader>
        <Heading size="md">
          Welcome to the OpenTelemetry Desktop Viewer.
        </Heading>
        <CardBody>
          <Stack spacing={3}>
            <Text>
              This lightweight [thingy] allows you to [insert a few more words
              here please]. Let's get you up and running:
            </Text>
            <Box>
              <OrderedList>
                <ListItem>Lorem ipsum dolor sit amet</ListItem>
                <ListItem>Consectetur adipiscing elit</ListItem>
                <ListItem>Integer molestie lorem at massa</ListItem>
                <ListItem>Facilisis in pretium nisl aliquet</ListItem>
              </OrderedList>
            </Box>
            <Text>
              Alternately, you can load some example data to get a feel for the
              tool.
            </Text>
          </Stack>
        </CardBody>
        <CardFooter>
          <SampleDataButton />
        </CardFooter>
      </Card>
    </Flex>
  );
}
