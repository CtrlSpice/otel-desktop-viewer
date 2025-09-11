import { writable } from "svelte/store"

// Check if we're in the browser
let browser = typeof window !== "undefined"

// Define the possible routes in our app
export type Route =
  | { page: "home" }
  | { page: "metrics" }
  | { page: "logs" }
  | { page: "traces" }

// Helper function to get route from URL path
function getRouteFromPath(path: string): Route {
  if (path === "/metrics") return { page: "metrics" }
  if (path === "/logs") return { page: "logs" }
  if (path === "/traces") return { page: "traces" }
  return { page: "home" }
}

// Helper function to get path from route
function getPathFromRoute(route: Route): string {
  if (route.page === "home") return "/"
  return `/${route.page}`
}

// Create the routing store with initial route from URL or default to home
let initialRoute: Route = { page: "home" }
if (browser) {
  initialRoute = getRouteFromPath(window.location.pathname)
}

export let currentRoute = writable<Route>(initialRoute)

// Helper function to navigate to a specific route
export function navigateTo(route: Route) {
  currentRoute.set(route)
}

// Helper functions for common navigation
export let navigate = {
  home: () => navigateTo({ page: "home" }),
  metrics: () => navigateTo({ page: "metrics" }),
  logs: () => navigateTo({ page: "logs" }),
  traces: () => navigateTo({ page: "traces" }),
}

// URL synchronization (only in browser)
if (browser) {
  // Listen for browser back/forward buttons
  window.addEventListener("popstate", () => {
    let route = getRouteFromPath(window.location.pathname)
    currentRoute.set(route)
  })

  // Update URL when route changes
  currentRoute.subscribe((route) => {
    let path = getPathFromRoute(route)
    if (window.location.pathname !== path) {
      window.history.pushState(null, "", path)
    }
  })
}
