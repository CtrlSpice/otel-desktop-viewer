import React from "react";
import { createRoot } from "react-dom/client";
import { ChakraProvider } from "@chakra-ui/react";
import { createBrowserRouter, RouterProvider } from "react-router-dom";
import { extendTheme, type ThemeConfig } from "@chakra-ui/react";

import MainView, { mainLoader } from "./routes/main-view";
import TraceView, { traceLoader } from "./routes/trace-view";
import LogsView from "./routes/logs-view";
import ErrorPage from "./error-page";

let config: ThemeConfig = {
  initialColorMode: "system",
  useSystemColorMode: true,
};
let theme = extendTheme({ config });

// Check feature flag for logs
let enableLogs = localStorage.getItem('enableLogs') === 'true';
console.log('Debug: enableLogs flag at module load:', enableLogs);

let routeChildren: any[] = [
  {
    path: "traces/:traceID",
    element: <TraceView />,
    loader: traceLoader,
  },
];

// Always add logs route, but we'll conditionally render it
routeChildren.push({
  path: "logs",
  element: <LogsView />,
});

console.log('Debug: routeChildren:', routeChildren);

let router = createBrowserRouter([
  {
    path: "/",
    element: <MainView />,
    loader: mainLoader,
    errorElement: <ErrorPage />,
    children: routeChildren,
  },
]);

let container = document.getElementById("root");
if (!!container) {
  let root = createRoot(container);

  root.render(
    <React.StrictMode>
      <ChakraProvider theme={theme}>
        <RouterProvider router={router} />
      </ChakraProvider>
    </React.StrictMode>,
  );
}
