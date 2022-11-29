import React from "react";
import { Outlet, NavLink, useLoaderData } from "react-router-dom";
import { FixedSizeList } from "react-window";
import {
  Flex,
  IconButton,
  Spacer,
  useBoolean,
  useColorModeValue,
} from "@chakra-ui/react";
import { ArrowLeftIcon, ArrowRightIcon } from "@chakra-ui/icons";

import { TraceSummaries, TraceSummary } from "../types/api-types";

export async function mainLoader() {
  const response = await fetch("/api/traces");
  const traceSummaries = await response.json();
  return traceSummaries;
}

type RowProps = {
  index: number;
  style: Object;
  data: TraceSummary[];
};

function Row({ index, style, data }: RowProps) {
  return (
    <NavLink
      to={`traces/${data[index].traceID}`}
      style={style}
    >
      {data[index].traceID}
    </NavLink>
  );
}

type SidebarProps = {
  isFullWidth: boolean;
  toggle: () => void;
};

function Sidebar(props: SidebarProps) {
  const sidebarColour = useColorModeValue("pink.100", "pink.900");
  const { traceSummaries } = useLoaderData() as TraceSummaries;

  let sidebarWidth = "80px";
  let buttonIcon = <ArrowRightIcon />;
  let traceList = <></>;

  if (props.isFullWidth) {
    sidebarWidth = "250px";
    buttonIcon = <ArrowLeftIcon />;
    traceList = (
      <FixedSizeList
        className="list"
        height={500}
        itemData={traceSummaries}
        itemCount={traceSummaries.length}
        itemSize={30}
        width="100%"
      >
        {Row}
      </FixedSizeList>
    );
  }

  return (
    <Flex
      bg={sidebarColour}
      direction="column"
      transition="width 0.2s ease-in-out"
      width={sidebarWidth}
    >
      <Flex justifyContent={"flex-end"}>
        <IconButton
          aria-label="Expand Sidebar"
          colorScheme="pink"
          icon={buttonIcon}
          margin="15px"
          onClick={() => props.toggle()}
        />
      </Flex>
      {traceList}
    </Flex>
  );
}

export default function MainView() {
  let [isFullWidth, setFullWidth] = useBoolean();

  return (
    <div className="container">
      <Sidebar
        isFullWidth={isFullWidth}
        toggle={setFullWidth.toggle}
      />
      <Outlet />
    </div>
  );
}
