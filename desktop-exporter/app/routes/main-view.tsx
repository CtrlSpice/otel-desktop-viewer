import React, { useState, useEffect } from "react";
import { Outlet } from "react-router-dom";
import { Flex, useBoolean } from "@chakra-ui/react";
import { useLoaderData } from "react-router-dom";

import { Sidebar } from "../components/sidebar-view/sidebar";
import { EmptyStateView } from "../components/empty-state-view/empty-state-view";
import { TraceSummaries, TraceSummary } from "../types/api-types";
import { SidebarData, TraceSummaryWithUIData } from "../types/ui-types";
import { getDurationNs, getDurationString } from "../utils/duration";

export async function mainLoader() {
  const response = await fetch("/api/traces");
  const traceSummaries = await response.json();
  return traceSummaries;
}

export default function MainView() {
  let { traceSummaries } = useLoaderData() as TraceSummaries;
  let [isFullWidth, setFullWidth] = useBoolean(traceSummaries.length > 0);
  // initialize the sidebar summaries at mount time
  let [sidebarData, setSidebarData] = useState(initSidebarData(traceSummaries));

  // check every second to see if we have new data and upsate sidebar summaries accordingly
  useEffect(() => {
    async function checkForNewData() {
      let response = await fetch("/api/traces");
      if (response.ok) {
        let { traceSummaries } = (await response.json()) as TraceSummaries;
        let newSidebarData = {
          ...updateSidebarData(sidebarData, traceSummaries),
        };
        setSidebarData(newSidebarData);
      }
    }

    let interval = setInterval(checkForNewData, 1000);

    return () => clearInterval(interval);
  }, []);

  // Handle empty state
  if (!traceSummaries.length) {
    return (
      <Flex height="100vh">
        <Sidebar
          isFullWidth={isFullWidth}
          toggleSidebarWidth={setFullWidth.toggle}
          traceSummaries={[]}
          numNewTraces={0}
        />
        <EmptyStateView />
      </Flex>
    );
  }

  return (
    <Flex height="100vh">
      <Sidebar
        isFullWidth={isFullWidth}
        toggleSidebarWidth={setFullWidth.toggle}
        traceSummaries={sidebarData.summaries}
        numNewTraces={sidebarData.numNewTraces}
      />
      <Outlet />
    </Flex>
  );
}

function initSidebarData(traceSummaries: TraceSummary[]): SidebarData {
  return {
    summaries: traceSummaries.map((traceSummary) =>
      generateTraceSummaryWithUIData(traceSummary),
    ),
    numNewTraces: 0,
  };
}

function updateSidebarData(
  sidebarData: SidebarData,
  traceSummaries: TraceSummary[],
): SidebarData {
  sidebarData.numNewTraces = 0;

  // Check for new and stale traces
  for (let i = 0; i < traceSummaries.length; i++) {
    let traceID = traceSummaries[i].traceID;
    let sidebarSummaryIndex = sidebarData.summaries.findIndex(
      (s) => s.traceID === traceID,
    );

    if (sidebarSummaryIndex === -1) {
      // If the traceID of the new summary has no match in the sidebar
      // increment the number of new traces.
      sidebarData.numNewTraces++;
    } else if (
      traceSummaries[i].spanCount >
      sidebarData.summaries[sidebarSummaryIndex].spanCount
    ) {
      // If the number of spans in an existing trace is greater than what's displayed in the sidebar
      // generate a whole new summary with ui data
      sidebarData.summaries[sidebarSummaryIndex] =
        generateTraceSummaryWithUIData(traceSummaries[i]);
    }
  }

  // Check for deleted/expired traces
  for (let i = 0; i < sidebarData.summaries.length; i++) {
    let traceID = sidebarData.summaries[i].traceID;
    let counterpartIndex = traceSummaries.findIndex(
      (s) => s.traceID === traceID,
    );
    if (counterpartIndex === -1) {
      // If a summary present in the sidebar is not present in the list of incoming traces
      // it is expired and must be removed
      sidebarData.summaries.splice(i, 1);
    }
  }
  return sidebarData;
}

function generateTraceSummaryWithUIData(
  traceSummary: TraceSummary,
): TraceSummaryWithUIData {
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
}
