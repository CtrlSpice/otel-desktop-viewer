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
import { getDurationNs, getDurationString } from "../../utils/duration";
import { StatusCodeString, SpanKindString } from "../../utils/enum-mapping";

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

  let durationString = getDurationString(
    getDurationNs(span.startTime, span.endTime),
  );

  let statusString = StatusCodeString[span.statusCode];

  let spanKindString = SpanKindString[span.kind];

  // Attributes:
  let spanAttributes = Object.entries(span.attributes).map(([key, value]) => (
    <li key={key}>
      <SpanField
        fieldName={key}
        fieldValue={value}
      />
    </li>
  ));

  let resourceAttributes = Object.entries(span.resource.attributes).map(
    ([key, value]) => (
      <li key={key}>
        <SpanField
          fieldName={key}
          fieldValue={value}
        />
      </li>
    ),
  );

  let scopeAttributes = Object.entries(span.scope.attributes).map(
    ([key, value]) => (
      <li key={key}>
        <SpanField
          fieldName={key}
          fieldValue={value}
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
            />
            <SpanField
              fieldName="kind"
              fieldValue={spanKindString}
            />
            <SpanField
              fieldName="start time"
              fieldValue={span.startTime}
            />
            <SpanField
              fieldName="end time"
              fieldValue={span.endTime}
            />
            <SpanField
              fieldName="duration"
              fieldValue={durationString}
            />
            <SpanField
              fieldName="status code"
              fieldValue={statusString}
            />
            <SpanField
              fieldName="status message"
              fieldValue={span.statusMessage}
              hidden={statusString === "unset" || statusString === "ok"}
            />
            <SpanField
              fieldName="trace id"
              fieldValue={span.traceID}
            />
            <SpanField
              fieldName="parent span id"
              fieldValue={span.parentSpanID}
              hidden={isRoot}
            />
            <SpanField
              fieldName="span id"
              fieldValue={span.spanID}
            />
            <List>{spanAttributes}</List>
            <SpanField
              fieldName="dropped attributes count"
              fieldValue={span.droppedAttributesCount}
              hidden={span.droppedAttributesCount === 0}
            />
            <SpanField
              fieldName="dropped events count"
              fieldValue={span.droppedEventsCount}
              hidden={span.droppedEventsCount === 0}
            />
            <SpanField
              fieldName="dropped links count"
              fieldValue={span.droppedLinksCount}
              hidden={span.droppedLinksCount === 0}
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
              fieldValue={span.resource.droppedAttributesCount}
              hidden={span.resource.droppedAttributesCount === 0}
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
            />
            <SpanField
              fieldName="scope version"
              fieldValue={span.scope.version}
            />
            <List>{scopeAttributes}</List>
            <SpanField
              fieldName="dropped attributes count"
              fieldValue={span.scope.droppedAttributesCount}
              hidden={span.scope.droppedAttributesCount === 0}
            />
          </AccordionPanel>
        </AccordionItem>
      </Accordion>
    </TabPanel>
  );
}
