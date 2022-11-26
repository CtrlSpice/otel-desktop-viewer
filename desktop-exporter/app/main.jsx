import React from "react";
import { createRoot } from "react-dom/client";
import { ChakraProvider } from "@chakra-ui/react";
import { createBrowserRouter, RouterProvider } from "react-router-dom";

import MainView, { mainLoader } from "./routes/main-view";
import TraceView, { traceLoader } from "./routes/trace-view";
import ErrorPage from "./error-page";

const router = createBrowserRouter([
  {
    path: "/",
    element: <MainView />,
    loader: mainLoader,
    errorElement: <ErrorPage />,
    children: [
      {
        path: "traces/:traceID",
        element: <TraceView />,
        loader: traceLoader,
      },
    ],
  },
]);

const container = document.getElementById("root");
const root = createRoot(container);

root.render(
  <React.StrictMode>
    <ChakraProvider>
      <RouterProvider router={router} />
    </ChakraProvider>
  </React.StrictMode>,
);
