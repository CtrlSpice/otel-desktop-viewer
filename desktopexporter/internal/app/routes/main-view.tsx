import React, { useState, useEffect } from "react";
import { Outlet } from "react-router-dom";
import { Flex, useBoolean } from "@chakra-ui/react";
import { useLoaderData } from "react-router-dom";

import { Sidebar } from "../components/sidebar-view/sidebar";
import { EmptyStateView } from "../components/empty-state-view/empty-state-view";
import { TraceSummary } from "../types/api-types";
import { SidebarData, TraceSummaryWithUIData } from "../types/ui-types";
import { telemetryAPI } from "../services/telemetry-service";

export async function mainLoader() {
  try {
    const traceSummaries = await telemetryAPI.getTraceSummaries();
    return traceSummaries;
  } catch (error) {
    console.error('Failed to load trace summaries:', error);
    return [];
  }
}

export default function MainView() {
  let traceSummaries = useLoaderData() as TraceSummary[];
  let [isFullWidth, setFullWidth] = useBoolean(traceSummaries.length > 0);

  // initialize the sidebar summaries at mount time
  let [sidebarData, setSidebarData] = useState(initSidebarData(traceSummaries));

  // check every second to see if we have new data
  // and update sidebar summaries accordingly
  useEffect(() => {
    async function checkForNewData() {
      try {
        let traceSummaries = await telemetryAPI.getTraceSummaries();
        let newSidebarData = updateSidebarData(sidebarData, traceSummaries);
        setSidebarData(newSidebarData);
      } catch (error) {
        console.error('Failed to check for new data:', error);
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
          traceSummaries={new Map()}
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
  const summaries = new Map<string, TraceSummaryWithUIData>();
  traceSummaries.forEach(summary => {
    summaries.set(summary.traceID, transformSummaryToUIData(summary));
  });

  return {
    summaries,
    numNewTraces: 0,
  };
}

function updateSidebarData(sidebarData: SidebarData, traceSummaries: TraceSummary[]): SidebarData {
  let mergedData: SidebarData = {
    numNewTraces: 0,
    summaries: new Map(sidebarData.summaries),
  };

  // First pass: Process new and updated traces
  for (let summary of traceSummaries) {
    let traceID = summary.traceID;
    let existingSummary = mergedData.summaries.get(traceID);
    
    if (!existingSummary) {
      // New trace
      mergedData.numNewTraces++;
      mergedData.summaries.set(traceID, transformSummaryToUIData(summary));
    } else if (summary.spanCount !== existingSummary.spanCount) {
      // Trace was updated (spans added or removed)
      mergedData.summaries.set(traceID, transformSummaryToUIData(summary));
    }
  }

  // Second pass: Remove deleted traces
  const currentTraceIDs = new Set(traceSummaries.map(s => s.traceID));
  for (let [traceID] of mergedData.summaries) {
    if (!currentTraceIDs.has(traceID)) {
      mergedData.summaries.delete(traceID);
    }
  }

  return mergedData;
}

function transformSummaryToUIData(traceSummary: TraceSummary): TraceSummaryWithUIData {
  if (traceSummary.rootSpan) {
    return {
      root: {
        ...traceSummary.rootSpan
      },
      spanCount: traceSummary.spanCount
    };
  }

  return {
    spanCount: traceSummary.spanCount
  };
}
