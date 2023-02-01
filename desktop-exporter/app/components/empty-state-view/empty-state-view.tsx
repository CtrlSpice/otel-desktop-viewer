import React, { useEffect, useState } from "react";
import {
  Alert,
  AlertDescription,
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
  useColorModeValue,
} from "@chakra-ui/react";

function RefreshAlert() {
  let alertColour = useColorModeValue("cyan.700", "cyan.300");
  let [secondsToRefresh, setSecondsToRefresh] = useState(10);
  useEffect(() => {
    setTimeout(() => {
      if (secondsToRefresh > 0) {
        setSecondsToRefresh(secondsToRefresh - 1);
      } else {
        window.location.reload();
      }
    }, 1000);
  });

  let alertText = "";
  if (secondsToRefresh > 1) {
    alertText = `No data yet. Refreshing in ${secondsToRefresh} seconds...`;
  } else if (secondsToRefresh === 1) {
    alertText = `No data yet. Refreshing in ${secondsToRefresh} second...`;
  } else {
    alertText = "No data yet. Refreshing now!";
  }

  return (
    <Alert
      status="info"
      variant="solid"
      minHeight="64px"
      backgroundColor={alertColour}
    >
      <AlertIcon boxSize="24px" />
      <AlertTitle fontSize="md">{alertText}</AlertTitle>
    </Alert>
  );
}

export function EmptyStateView() {
  return (
    <Flex
      flexDirection="column"
      align="center"
      width="100%"
      overflowY="scroll"
    >
      <RefreshAlert />
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
          <Button colorScheme="pink">Load Sample Data</Button>
        </CardFooter>
      </Card>
    </Flex>
  );
}
