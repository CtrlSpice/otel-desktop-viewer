import React from "react";
import { Flex, Tab, TabList, TabPanels, Tabs } from "@chakra-ui/react";

import { SpanData } from "../../types/api-types";
import { FieldsPannel } from "./fields-pannel";

type DetailViewProps = {
  span: SpanData | undefined;
};

export function DetailView(props: DetailViewProps) {
  let { span } = props;
  if (!span) {
    return <div></div>;
  }
  let numEvents = span.events.length;
  let numLinks = span.links.length;

  console.log(span);
  return (
    <Flex
      grow="0"
      shrink="1"
      basis="350px"
      height="100vh"
    >
      <Tabs
        colorScheme="pink"
        margin={3}
        overflowY="scroll"
        size="sm"
        variant="soft-rounded"
        width="100vw"
      >
        <TabList>
          <Tab>Fields</Tab>
          {numEvents ? (
            <Tab>Events({numEvents})</Tab>
          ) : (
            <Tab isDisabled>Events(0)</Tab>
          )}
          {numLinks ? (
            <Tab>Links({numLinks})</Tab>
          ) : (
            <Tab isDisabled>Links(0)</Tab>
          )}
        </TabList>
        <TabPanels>
          <FieldsPannel span={span} />
        </TabPanels>
      </Tabs>
    </Flex>
  );
}
