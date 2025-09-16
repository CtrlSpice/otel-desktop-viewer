import { writable } from 'svelte/store';

// Types
export type Route =
  | { page: 'home' }
  | { page: 'metrics' }
  | { page: 'logs' }
  | { page: 'traces' };

// Configuration
let browser = typeof window !== 'undefined';

// Route Parsing
function getRouteFromPath(path: string): Route {
  switch (path) {
    case '/metrics':
      return { page: 'metrics' };
    case '/logs':
      return { page: 'logs' };
    case '/traces':
      return { page: 'traces' };
    default:
      return { page: 'home' };
  }
}

function getPathFromRoute(route: Route): string {
  return route.page === 'home' ? '/' : `/${route.page}`;
}

// Store
function getInitialRoute(): Route {
  if (browser) {
    return getRouteFromPath(window.location.pathname);
  }
  return { page: 'home' };
}

export let currentRoute = writable<Route>(getInitialRoute());

// Navigation
export function navigateTo(route: Route) {
  currentRoute.set(route);
}

export let navigate = {
  home: () => navigateTo({ page: 'home' }),
  metrics: () => navigateTo({ page: 'metrics' }),
  logs: () => navigateTo({ page: 'logs' }),
  traces: () => navigateTo({ page: 'traces' }),
};

// URL Synchronization
if (browser) {
  // Handle browser back/forward buttons
  window.addEventListener('popstate', () => {
    let route = getRouteFromPath(window.location.pathname);
    currentRoute.set(route);
  });

  // Update URL when route changes
  currentRoute.subscribe(route => {
    let path = getPathFromRoute(route);
    if (window.location.pathname !== path) {
      window.history.pushState(null, '', path);
    }
  });
}
