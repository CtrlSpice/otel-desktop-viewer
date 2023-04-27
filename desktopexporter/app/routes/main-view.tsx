import React from "react";
import { Outlet } from "react-router-dom";
import { Flex, useBoolean } from "@chakra-ui/react";
import { useLoaderData } from "react-router-dom";

import { Sidebar } from "../components/sidebar-view/sidebar";
import { EmptyStateView } from "../components/empty-state-view/empty-state-view";
import { TraceSummaries, TraceSummary } from "../types/api-types";
import { TraceSummaryWithUIData } from "../types/ui-types";
import { getDurationNs, getDurationString } from "../utils/duration";

export async function mainLoader() {
  const response = await fetch("/api/traces");
  const traceSummaries = await response.json();
  return traceSummaries;
}

export default function MainView() {
  let { traceSummaries } = useLoaderData() as TraceSummaries;
  let [isFullWidth, setFullWidth] = useBoolean(traceSummaries.length > 0);

  // Handle empty state
  if (!traceSummaries.length) {
    return (
      <Flex height="100vh">
        <Sidebar
          isFullWidth={isFullWidth}
          toggleSidebarWidth={setFullWidth.toggle}
          traceSummaries={[]}
        />
        <EmptyStateView />
      </Flex>
    );
  }

  let sidebarSummaries: TraceSummaryWithUIData[] =
    getTraceSummariesWithUIData(traceSummaries);
  return (
    <Flex height="100vh">
      <Sidebar
        isFullWidth={isFullWidth}
        toggleSidebarWidth={setFullWidth.toggle}
        traceSummaries={sidebarSummaries}
      />
      <Outlet />
    </Flex>
  );
}

function getTraceSummariesWithUIData(
  traceSummaries: TraceSummary[],
): TraceSummaryWithUIData[] {
  return traceSummaries.map((traceSummary) => {
    if (traceSummary.hasRootSpan) {
      let duration = getDurationNs(
        traceSummary.rootStartTime,
        traceSummary.rootEndTime,
      );

      let durationString = getDurationString(duration);
      return {
        hasRootSpan: true,
        rootServiceName: traceSummary.rootServiceName,
        rootName: traceSummary.rootName,
        rootDurationString: durationString,
        spanCount: traceSummary.spanCount,
        traceID: traceSummary.traceID,
      };
    }
    return {
      hasRootSpan: false,
      spanCount: traceSummary.spanCount,
      traceID: traceSummary.traceID,
    };
  });
}