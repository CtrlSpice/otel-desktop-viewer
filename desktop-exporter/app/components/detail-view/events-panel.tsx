import React from "react";
import {
  Accordion,
  AccordionButton,
  AccordionIcon,
  AccordionItem,
  AccordionPanel,
  Box,
  Heading,
  List,
  TabPanel,
  Text,
} from "@chakra-ui/react";

import { EventData } from "../../types/api-types";
import { SpanField } from "./span-field";
import { getDurationString } from "../../utils/duration";

type EventsPanelProps = {
  events: EventData[] | undefined;
  spanStartTime: string;
};

export function EventsPanel(props: EventsPanelProps) {
  console.log(props);
  let { events, spanStartTime } = props;
  if (!events) {
    return null;
  }

  let eventList = events.map((event) => {
    let timeSinceSpanStart = getDurationString(spanStartTime, event.timestamp);
    let eventAttributes = Object.entries(event.attributes).map(
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
      <li key={event.name + event.timestamp}>
        <AccordionItem>
          <AccordionButton>
            <Box
              flex="1"
              textAlign="left"
            >
              <Heading size="sm">{event.name}</Heading>
              <Text fontSize="xs">{timeSinceSpanStart} since span start</Text>
            </Box>
            <AccordionIcon />
          </AccordionButton>
          <AccordionPanel>
            <SpanField
              fieldName="timestamp"
              fieldValue={event.timestamp}
            />
            <List>{eventAttributes}</List>
            <SpanField
              fieldName="dropped attributes count"
              fieldValue={event.droppedAttributeCount}
              hidden={!event.droppedAttributeCount}
            />
          </AccordionPanel>
        </AccordionItem>
      </li>
    );
  });

  return (
    <TabPanel paddingX="0px">
      <Accordion allowMultiple>
        <List>{eventList}</List>
      </Accordion>
    </TabPanel>
  );
}
