// TypeScript declarations for tinro5 router
// tinro5 doesn't ship with its own TypeScript definitions, so we provide them here
declare module 'tinro5' {
  import { SvelteComponent } from 'svelte';

  export interface RouteProps {
    path: string;
    children?: any;
  }

  export class Route extends SvelteComponent<RouteProps> {}

  export const router: {
    goto: (path: string) => void;
    mode: {
      history: () => void;
      hash: () => void;
      memory: () => void;
    };
    path: string;
    subscribe: (
      callback: (route: {
        path: string;
        url: string;
        from?: string;
        hash: string;
        query: any;
      }) => void
    ) => () => void;
  };
}
