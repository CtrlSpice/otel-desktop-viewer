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
  Spacer,
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
        size="sm"
        spinnerPlacement="start"
        width="fit-content"
      />
    );
  }

  return (
    <Button
      colorScheme="pink"
      size="sm"
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
      window.location.reload();
    }
  }
}

export function EmptyStateView() {
  let codeBackgroundColour = useColorModeValue(
    "blackAlpha.300",
    "whiteAlpha.300",
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
        width="80%"
        marginY="64px"
        minWidth="750px"
        padding="40px"
        variant="filled"
      >
        <CardBody>
          <Flex width="100%">
            <Image
              src="assets/images/lulu.jpg"
              alt="A pink axolotl is striking a heroic pose while gazing at a field of stars through a telescope. Her name is Lulu Axol'Otel the First, valiant adventurer and observability queen."
              maxWidth="500px"
              borderRadius="lg"
            />
            <Stack
              spacing={3}
              justifyContent="center"
              marginX="24px"
            >
              <Heading size="lg">
                Welcome to the OpenTelemetry Desktop Viewer.
              </Heading>
              <Divider />
              <Text>
                This CLI tool allows you to receive OpenTelemetry traces while
                working on your local machine, helping you visualize and explore
                your trace data without needing to send it on to a telemetry
                vendor.
              </Text>
              <Heading size="md">Example with Sample Data</Heading>
              <Divider />
              <Text>
                If you would like to explore the application without sending it
                anything, you can do so by loading some sample data.
              </Text>
              <SampleDataButton />
            </Stack>
          </Flex>

          <Stack
            spacing={3}
            marginTop="30px"
          >
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
            <Heading size="md">Example with Otel-Cli</Heading>
            <Divider />
            <Text>
              If you have{" "}
              <Code backgroundColor={codeBackgroundColour}>
                <Link
                  color="teal.400"
                  href="https://github.com/equinix-labs/otel-cli"
                  isExternal
                >
                  otel-cli
                </Link>
              </Code>{" "}
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
            Made with ✨ by{" "}
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
