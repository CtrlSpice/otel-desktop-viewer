import React from "react";
import {
  Alert,
  AlertIcon,
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
} from "@chakra-ui/react";

export function EmptyStateView() {
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
        minHeight="30px"
      >
        <AlertIcon />
        No data yet. Checking again in 15 seconds...
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
            alt="An axolotl named Lulu, striking a heroic pose while gazing at a field of stars through a telescope"
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
          <Button colorScheme="pink">Load Example Data</Button>
        </CardFooter>
      </Card>
    </Flex>
  );
}
