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
import { parseAttributeType } from "../../utils/parse-type";
import { PreciseTimestamp } from "../../types/precise-timestamp";
import { getDuration } from "../../utils/duration";

type EventItemProps = {
  event: EventData;
  spanStartTime: PreciseTimestamp;
};

function EventItem(props: EventItemProps) {
  let { event, spanStartTime } = props;
  let timeSinceSpanStart = getDuration(spanStartTime, event.timestamp).label;

  let eventAttributes = Object.entries(event.attributes).map(([key, value]) => (
    <li key={key + value?.toString()}>
      <SpanField
        fieldName={key}
        fieldValue={value.toString()}
        fieldType={parseAttributeType(value)}
      />
    </li>
  ));

  return (
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
          fieldValue={event.timestamp.toString()}
          fieldType="timestamp"
        />
        <List>{eventAttributes}</List>
        <SpanField
          fieldName="dropped attributes count"
          fieldValue={event.droppedAttributesCount.toString()}
          fieldType="uint32"
          hidden={!event.droppedAttributesCount}
        />
      </AccordionPanel>
    </AccordionItem>
  );
}

type EventsPanelProps = {
  events: EventData[] | undefined;
  spanStartTime: PreciseTimestamp;
};

export function EventsPanel(props: EventsPanelProps) {
  let { events, spanStartTime } = props;
  if (!events) {
    return null;
  }

  let eventItemList = events.map((event) => (
    <li key={event.name + event.timestamp}>
      <EventItem
        event={event}
        spanStartTime={spanStartTime}
      />
    </li>
  ));

  return (
    <TabPanel paddingX="0px">
      <Accordion allowMultiple>
        <List>{eventItemList}</List>
      </Accordion>
    </TabPanel>
  );
}
