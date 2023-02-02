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

type LinkItemProps = {
  link: LinkData;
};

function LinkItem(props: LinkItemProps) {
  let { link } = props;
  let linkAttributes = Object.entries(link.attributes).map(([key, value]) => (
    <li key={key + value?.toString}>
      <SpanField
        fieldName={key}
        fieldValue={value}
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
        />
      </AccordionPanel>
    </AccordionItem>
  );
}

type LinksPanelProps = {
  links: LinkData[] | undefined;
};

export function LinksPanel(props: LinksPanelProps) {
  let { links } = props;
  if (!links) {
    return null;
  }

  let linkItemList = links.map((link) => (
    <li key={link.traceID + link.spanID}>
      <LinkItem link={link} />
    </li>
  ));

  return (
    <TabPanel paddingX="0px">
      <UnderConstructionAlert />
      <Accordion allowMultiple>
        <List>{linkItemList}</List>
      </Accordion>
    </TabPanel>
  );
}
