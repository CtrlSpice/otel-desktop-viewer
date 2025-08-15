import React from "react";
import { createRoot } from "react-dom/client";
import { ChakraProvider } from "@chakra-ui/react";
import { createBrowserRouter, RouterProvider } from "react-router-dom";
import { extendTheme, type ThemeConfig } from "@chakra-ui/react";

import MainView, { mainLoader } from "./routes/main-view";
import TraceView, { traceLoader } from "./routes/trace-view";
import LogsView from "./routes/logs-view";
import MetricsView from "./routes/metrics-view";
import ErrorPage from "./error-page";

let config: ThemeConfig = {
  initialColorMode: "system",
  useSystemColorMode: true,
};
let theme = extendTheme({ config });

let routeChildren: any[] = [
  {
    path: "traces",     // /traces
    element: <TraceView />,
    loader: traceLoader,
  },
  {
    path: "traces/:traceID",     // /traces/{id}
    element: <TraceView />,
    loader: traceLoader,
  },
  {
    path: "logs",               // /logs
    element: <LogsView />,
  },
  {
    path: "metrics",            // /metrics
    element: <MetricsView />,
  },
];

let router = createBrowserRouter([
  {
    path: "/",                    // Root path
    element: <MainView />,        // Main layout component
    loader: mainLoader,           // Data loader for traces
    errorElement: <ErrorPage />,  // Error boundary
    children: routeChildren,      // Child routes (traces, logs, metrics)
  },
]);

let container = document.getElementById("root");
if (!!container) {
  let root = createRoot(container);

  root.render(
    <React.StrictMode>
      <ChakraProvider theme={theme}>
        <RouterProvider router={router} />  // ← Router injected here
      </ChakraProvider>
    </React.StrictMode>,
  );
}
