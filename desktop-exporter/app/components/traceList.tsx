import React, { useRef } from "react";
import { FixedSizeList } from "react-window";
import { NavLink, useLoaderData } from "react-router-dom";
import { Flex } from "@chakra-ui/react";
import { useSize } from "@chakra-ui/react-use-size";

import { TraceSummaries, TraceSummary } from "../types/api-types";

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

export function TraceList() {
  const { traceSummaries } = useLoaderData() as TraceSummaries;
  const ref = useRef(null);
  const size = useSize(ref);

  return (
    <Flex
      ref={ref}
      height="100%"
    >
      <FixedSizeList
        className="list"
        height={size ? size.height : 0}
        itemData={traceSummaries}
        itemCount={traceSummaries.length}
        itemSize={50}
        width="100%"
      >
        {Row}
      </FixedSizeList>
    </Flex>
  );
}
