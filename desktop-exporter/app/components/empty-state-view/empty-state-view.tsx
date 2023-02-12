import React from "react";
import {
  Button,
  Card,
  CardBody,
  CardFooter,
  CardHeader,
  Code,
  Divider,
  Flex,
  Heading,
  Image,
  Link,
  Stack,
  Text,
  useBoolean,
  useColorModeValue,
  useInterval,
} from "@chakra-ui/react";

import { TraceSummaries } from "../../types/api-types";

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
        width="fit-content"
      />
    );
  }

  return (
    <Button
      colorScheme="pink"
      width="fit-content"
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
  let response = await fetch("/api/traces");
  if (!response.ok) {
    throw new Error("HTTP status " + response.status);
  } else {
    let { traceSummaries } = (await response.json()) as TraceSummaries;
    if (traceSummaries.length > 0) {
      setTimeout(() => {
        window.location.reload();
      }, 500);
    }
  }
}

export function EmptyStateView() {
  let cardBackgroundColour = useColorModeValue("gray.200", "gray.700");
  let codeBackgroundColour = useColorModeValue(
    "whiteAlpha.700",
    "blackAlpha.500",
  );
  useInterval(pollTraceCount, 500);

  return (
    <Flex
      flexDirection="column"
      align="center"
      justifyItems="center"
      width="100%"
      overflowY="scroll"
    >
      <Card
        align="center"
        backgroundColor={cardBackgroundColour}
        marginY="64px"
        minWidth="960px"
        padding="30px"
        variant="filled"
        width="60%"
      >
        <CardHeader
          borderRadius="lg"
          paddingY="0"
        >
          <Flex width="100%">
            <Image
              src="assets/images/lulu.png"
              alt="A pink axolotl is striking a heroic pose while gazing at a field of stars through a telescope. Her name is Lulu Axol'Otel the First, valiant adventurer and observability queen."
              maxHeight="400px"
              maxWidth="400px"
              borderRadius="lg"
            />
            <Flex
              marginLeft="24px"
              justifyContent="flex-end"
              direction="column"
            >
              <Stack spacing={3}>
                <Heading size="lg">
                  Welcome to the OpenTelemetry Desktop Viewer.
                </Heading>
                <Divider />
                <Text>
                  This CLI tool allows you to receive OpenTelemetry traces while
                  working on your local machine, helping you visualize and
                  explore your trace data without needing to send it on to a
                  telemetry vendor.
                </Text>
              </Stack>
              <Stack
                spacing={3}
                marginTop="24px"
              >
                <Heading size="md">Explore with Sample Data</Heading>
                <Divider />
                <Text>
                  If you would like to explore the application without sending
                  it anything, you can do so by loading some sample data.
                </Text>
                <SampleDataButton />
              </Stack>
            </Flex>
          </Flex>
        </CardHeader>
        <CardBody>
          <Stack spacing={3}>
            <Heading size="md">Configuring your OpenTelemetry SDK</Heading>
            <Divider />
            <Text>
              To send telemetry to OpenTelemetry Desktop Viewer from your
              application, you need to configure an OTLP exporter to send via
              grpc to{" "}
              <Code backgroundColor={codeBackgroundColour}>
                http://localhost:4317
              </Code>{" "}
              or via http to{" "}
              <Code backgroundColor={codeBackgroundColour}>
                http://localhost:4318
              </Code>
              .
            </Text>
            <Text>
              If your OpenTelemetry SDK OTLP exporter supports{" "}
              <Link
                color="teal.500"
                href="https://opentelemetry.io/docs/concepts/sdk-configuration/otlp-exporter-configuration/"
                isExternal
              >
                configuration via environment variables{" "}
              </Link>{" "}
              then you should be able to send to{" "}
              <Code backgroundColor={codeBackgroundColour}>
                otel-desktop-viewer
              </Code>{" "}
              with the following environment variables set.
            </Text>
            <Heading size="sm">For HTTP:</Heading>
            <Flex
              backgroundColor={codeBackgroundColour}
              padding={2}
            >
              <Code
                backgroundColor="transparent"
                display="block"
                whiteSpace="pre"
                children={`export OTEL_EXPORTER_OTLP_ENDPOINT="http://localhost:4318"
export OTEL_TRACES_EXPORTER="otlp"
export OTEL_EXPORTER_OTLP_PROTOCOL="http/protobuf"`}
              ></Code>
            </Flex>
            <Heading size="sm">For GRPC:</Heading>
            <Flex
              backgroundColor={codeBackgroundColour}
              padding={2}
            >
              <Code
                backgroundColor="transparent"
                display="block"
                whiteSpace="pre"
                children={`export OTEL_EXPORTER_OTLP_ENDPOINT="http://localhost:4317"
export OTEL_TRACES_EXPORTER="otlp"
export OTEL_EXPORTER_OTLP_PROTOCOL="grpc"`}
              ></Code>
            </Flex>
          </Stack>
          <Stack
            spacing={3}
            marginTop="30px"
          >
            <Heading size="md">Example with otel-cli</Heading>
            <Divider />
            <Text>
              If you have{" "}
              <Link
                color="teal.400"
                href="https://github.com/equinix-labs/otel-cli"
                isExternal
              >
                otel-cli
              </Link>{" "}
              installed, you can send some example data with the following
              script.
            </Text>
            <Flex
              backgroundColor={codeBackgroundColour}
              padding={2}
            >
              <Code
                backgroundColor="transparent"
                display="block"
                whiteSpace="pre"
                children={`# start the desktop viewer (best to do this in a separate terminal)
otel-desktop-viewer

# configure otel-cli to send to our desktop viewer endpoint
export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318

# use otel-cli to generate spans!
otel-cli exec --service my-service --name "curl google" curl https://google.com
`}
              ></Code>
            </Flex>
          </Stack>
        </CardBody>
        <CardFooter alignItems="center">
          <Text>
            Made with{" "}
            <Image
              src="assets/images/axolotl.svg"
              alt="axolotl emoji"
              display="inline"
              width="30px"
            />{" "}
            by{" "}
            <Link
              color="teal.500"
              href="https://github.com/CtrlSpice"
              isExternal
            >
              Mila Ardath
            </Link>
            , with Artwork by{" "}
            <Link
              color="teal.500"
              href="https://cbatesonart.artstation.com/"
              isExternal
            >
              Chelsey Bateson
            </Link>
          </Text>
        </CardFooter>
      </Card>
    </Flex>
  );
}
