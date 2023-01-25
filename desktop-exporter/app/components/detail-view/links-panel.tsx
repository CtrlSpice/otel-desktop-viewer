import React from "react";

import {
  Accordion,
  AccordionButton,
  AccordionIcon,
  AccordionItem,
  AccordionPanel,
  Box,
  List,
  TabPanel,
  Text,
} from "@chakra-ui/react";

import { LinkData } from "../../types/api-types";
import { SpanField } from "./span-field";
import { UnderConstructionAlert } from "../alerts/under-construction";

type LinksPanelProps = {
  links: LinkData[] | undefined;
};

export function LinksPanel(props: LinksPanelProps) {
  let { links } = props;
  if (!links) {
    return null;
  }

  // As in a list of links, not the data structure. I'm sorry. Names are hard.
  let linkList = links.map((link) => {
    let linkAttributes = Object.entries(link.attributes).map(([key, value]) => (
      <li key={key + value}>
        <SpanField
          fieldName={key}
          fieldValue={value}
        />
      </li>
    ));

    return (
      <li key={link.traceID + link.spanID}>
        <AccordionItem>
          <AccordionButton>
            <Box
              flex="1"
              textAlign="left"
            >
              <Text fontSize="sm">
                Trace ID: <strong>{link.traceID}</strong>
              </Text>
              <Text fontSize="sm">
                Span ID: <strong>{link.spanID}</strong>
              </Text>
            </Box>
            <AccordionIcon />
          </AccordionButton>
          <AccordionPanel>
            <SpanField
              fieldName="trace state"
              fieldValue={link.traceState}
            />
            <List>{linkAttributes}</List>
            <SpanField
              fieldName="dropped attributes count"
              fieldValue={link.droppedAttributesCount}
              hidden={!link.droppedAttributesCount}
            />
          </AccordionPanel>
        </AccordionItem>
      </li>
    );
  });

  return (
    <TabPanel paddingX="0px">
      <UnderConstructionAlert />
      <Accordion allowMultiple>
        <List>{linkList}</List>
      </Accordion>
    </TabPanel>
  );
}
