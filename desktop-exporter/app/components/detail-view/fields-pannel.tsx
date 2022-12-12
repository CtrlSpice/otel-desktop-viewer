import React from "react";
import {
  Accordion,
  AccordionItem,
  AccordionButton,
  AccordionPanel,
  TabPanel,
  Box,
  AccordionIcon,
  Divider,
  List,
  Heading,
} from "@chakra-ui/react";

import { SpanData } from "../../types/api-types";
import { SpanField } from "./span-field";
import { getDurationString } from "../../utils/duration";

type FieldsPannelProps = {
  span: SpanData | undefined;
};

export function FieldsPannel(props: FieldsPannelProps) {
  let { span } = props;
  if (!span) {
    return (
      <TabPanel>
        <p>Nothing here yet.</p>
      </TabPanel>
    );
  }
  let durationString = getDurationString(span.startTime, span.endTime);

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
              <Heading size="sm">Span Data</Heading>
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
              fieldValue={span.kind}
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
            <Divider />

            <SpanField
              fieldName="status code"
              fieldValue={span.statusCode}
            />
            <SpanField
              fieldName="status message"
              fieldValue={span.statusMessage}
            />
            <Divider />

            <SpanField
              fieldName="trace id"
              fieldValue={span.traceID}
            />
            <SpanField
              fieldName="span id"
              fieldValue={span.spanID}
            />
            <SpanField
              fieldName="parent span id"
              fieldValue={span.parentSpanID}
            />
            <Divider />

            <SpanField
              fieldName="dropped attributes count"
              fieldValue={span.droppedAttributesCount}
            />
            <SpanField
              fieldName="dropped events count"
              fieldValue={span.droppedEventsCount}
            />
            <SpanField
              fieldName="dropped links count"
              fieldValue={span.droppedLinksCount}
            />
            <Divider />

            <List>{spanAttributes}</List>
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
            <SpanField
              fieldName="dropped attributes count"
              fieldValue={span.resource.droppedAttributesCount}
            />
            <Divider />

            <List>{resourceAttributes}</List>
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
            <SpanField
              fieldName="scope version"
              fieldValue={span.scope.version}
            />
            <SpanField
              fieldName="dropped attributes count"
              fieldValue={span.scope.droppedAttributesCount}
            />
            <Divider />

            <List>{scopeAttributes}</List>
          </AccordionPanel>
        </AccordionItem>
      </Accordion>
    </TabPanel>
  );
}
