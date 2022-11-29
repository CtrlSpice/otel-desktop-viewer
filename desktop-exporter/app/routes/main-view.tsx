import React from "react";
import { Outlet } from "react-router-dom";
import { useBoolean } from "@chakra-ui/react";

import { Sidebar } from "../components/sidebar";

export async function mainLoader() {
  const response = await fetch("/api/traces");
  const traceSummaries = await response.json();
  return traceSummaries;
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
