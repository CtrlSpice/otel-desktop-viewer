import React from "react";
import { Outlet } from "react-router-dom";
import { useBoolean } from "@chakra-ui/react";
import { useLoaderData } from "react-router-dom";

import { Sidebar } from "../components/sidebar";
import { TraceSummaries } from "../types/api-types";

export async function mainLoader() {
  const response = await fetch("/api/traces");
  const traceSummaries = await response.json();
  return traceSummaries;
}

export default function MainView() {
  let [isFullWidth, setFullWidth] = useBoolean();
  let { traceSummaries } = useLoaderData() as TraceSummaries;

  return (
    <div className="container">
      <Sidebar
        isFullWidth={isFullWidth}
        toggle={setFullWidth.toggle}
        traceSummaries={traceSummaries}
      />
      <Outlet />
    </div>
  );
}
