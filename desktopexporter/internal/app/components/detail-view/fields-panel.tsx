import React from "react";
import {
  Accordion,
  AccordionItem,
  AccordionButton,
  AccordionPanel,
  TabPanel,
  Box,
  AccordionIcon,
  List,
  Heading,
  Tag,
} from "@chakra-ui/react";

import { SpanData } from "../../types/api-types";
import { SpanField } from "./span-field";
import { formatDuration } from "../../utils/duration";
import { parseAttributeType } from "../../utils/parse-type";

type FieldsPanelProps = {
  span: SpanData | undefined;
};

export function FieldsPanel(props: FieldsPanelProps) {
  let { span } = props;
  if (!span) {
    return (
      <TabPanel>
        <p>Nothing here yet.</p>
      </TabPanel>
    );
  }

  // Root span: label with a little tag
  let isRoot = span.parentSpanID.length ? false : true;
  let rootTag = isRoot ? (
    <Tag
      marginStart={2}
      colorScheme="cyan"
      variant="subtle"
    >
      root
    </Tag>
  ) : null;

  // Duration: label in appropriate human-readable time unit (s, ms, μs, ns)
  let durationLabel = formatDuration(span.endTime.nanoseconds - span.startTime.nanoseconds);

  // Attributes:
  let spanAttributes = Object.entries(span.attributes).map(([key, value]) => (
    <li key={key}>
      <SpanField
        fieldName={key}
        fieldValue={value.toString()}
        fieldType={parseAttributeType(value)}
      />
    </li>
  ));

  let resourceAttributes = Object.entries(span.resource.attributes).map(
    ([key, value]) => (
      <li key={key}>
        <SpanField
          fieldName={key}
          fieldValue={value.toString()}
          fieldType={parseAttributeType(value)}
        />
      </li>
    ),
  );

  let scopeAttributes = Object.entries(span.scope.attributes).map(
    ([key, value]) => (
      <li key={key}>
        <SpanField
          fieldName={key}
          fieldValue={value.toString()}
          fieldType={parseAttributeType(value)}
        />
      </li>
    ),
  );

  return (
    <TabPanel paddingX="0px">
      <Accordion
        defaultIndex={[0]}
        allowMultiple
      >
        <AccordionItem>
          <AccordionButton>
            <Box
              flex="1"
              textAlign="left"
            >
              <Heading
                lineHeight="revert"
                size="sm"
              >
                Span Data{rootTag}
              </Heading>
            </Box>
            <AccordionIcon />
          </AccordionButton>
          <AccordionPanel>
            <SpanField
              fieldName="name"
              fieldValue={span.name}
              fieldType="string"
            />
            <SpanField
              fieldName="kind"
              fieldValue={span.kind}
              fieldType="string"
            />
            <SpanField
              fieldName="start time"
              fieldValue={span.startTime.toString()}
              fieldType="timestamp"
            />
            <SpanField
              fieldName="end time"
              fieldValue={span.endTime.toString()}
              fieldType="timestamp"
            />
            <SpanField
              fieldName="duration"
              fieldValue={durationLabel}
              fieldType="string"
            />
            <SpanField
              fieldName="status code"
              fieldValue={span.statusCode}
              fieldType="string"
            />
            <SpanField
              fieldName="status message"
              fieldValue={span.statusMessage}
              hidden={span.statusCode === "Unset" || span.statusCode === "Ok"}
              fieldType="string"
            />
            <SpanField
              fieldName="trace id"
              fieldValue={span.traceID}
              fieldType="string"
            />
            <SpanField
              fieldName="parent span id"
              fieldValue={span.parentSpanID}
              hidden={isRoot}
              fieldType="string"
            />
            <SpanField
              fieldName="span id"
              fieldValue={span.spanID}
              fieldType="string"
            />
            <List>{spanAttributes}</List>
            <SpanField
              fieldName="dropped attributes count"
              fieldValue={span.droppedAttributesCount.toString()}
              hidden={span.droppedAttributesCount === 0}
              fieldType="uint32"
            />
            <SpanField
              fieldName="dropped events count"
              fieldValue={span.droppedEventsCount.toString()}
              hidden={span.droppedEventsCount === 0}
              fieldType="uint32"
            />
            <SpanField
              fieldName="dropped links count"
              fieldValue={span.droppedLinksCount.toString()}
              hidden={span.droppedLinksCount === 0}
              fieldType="uint32"
            />
          </AccordionPanel>
        </AccordionItem>
        <AccordionItem>
          <AccordionButton>
            <Box
              flex="1"
              textAlign="left"
            >
              <Heading size="sm">Resource Data</Heading>
            </Box>
            <AccordionIcon />
          </AccordionButton>
          <AccordionPanel>
            <List>{resourceAttributes}</List>
            <SpanField
              fieldName="dropped attributes count"
              fieldValue={span.resource.droppedAttributesCount.toString()}
              hidden={span.resource.droppedAttributesCount === 0}
              fieldType="uint32"
            />
          </AccordionPanel>
        </AccordionItem>
        <AccordionItem>
          <AccordionButton>
            <Box
              flex="1"
              textAlign="left"
            >
              <Heading size="sm">Scope Data</Heading>
            </Box>
            <AccordionIcon />
          </AccordionButton>
          <AccordionPanel>
            <SpanField
              fieldName="scope name"
              fieldValue={span.scope.name}
              fieldType="string"
            />
            <SpanField
              fieldName="scope version"
              fieldValue={span.scope.version}
              fieldType="string"
            />
            <List>{scopeAttributes}</List>
            <SpanField
              fieldName="dropped attributes count"
              fieldValue={span.scope.droppedAttributesCount.toString()}
              hidden={span.scope.droppedAttributesCount === 0}
              fieldType="uint32"
            />
          </AccordionPanel>
        </AccordionItem>
      </Accordion>
    </TabPanel>
  );
}