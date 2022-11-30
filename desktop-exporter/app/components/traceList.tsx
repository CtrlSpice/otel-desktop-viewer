import React, { useRef } from "react";
import { FixedSizeList } from "react-window";
import { NavLink } from "react-router-dom";
import { Flex } from "@chakra-ui/react";
import { useSize } from "@chakra-ui/react-use-size";

import { TraceSummary } from "../types/api-types";

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

type TraceListProps = {
  traceSummaries: TraceSummary[];
};

export function TraceList(props: TraceListProps) {
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
        itemData={props.traceSummaries}
        itemCount={props.traceSummaries.length}
        itemSize={50}
        width="100%"
      >
        {Row}
      </FixedSizeList>
    </Flex>
  );
}
