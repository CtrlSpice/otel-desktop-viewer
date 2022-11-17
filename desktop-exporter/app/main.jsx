import React from 'react';
import { createRoot } from 'react-dom/client';

import {
  createBrowserRouter,
  RouterProvider,
  Route,
} from "react-router-dom";
import MainView, { mainLoader } from './routes/main-view';
import TraceView, { traceLoader } from "./routes/trace-view";
import ErrorPage from './error-page';

const router = createBrowserRouter([
  {
    path: "/",
    element: <MainView />,
    loader: mainLoader,
    errorElement: <ErrorPage />,
  },
  {
    path: "traces/:traceID",
    element: <TraceView />,
    loader: traceLoader,
    errorElement: <ErrorPage />,
  }
]);

const container = document.getElementById('root');
const root = createRoot(container);

root.render(
  <React.StrictMode>
    <RouterProvider router={router} />
  </React.StrictMode>
);