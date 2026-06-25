// TypeScript declarations for tinro5 router
// tinro5 doesn't ship with its own TypeScript definitions, so we provide them here
declare module 'tinro5' {
  import { SvelteComponent } from 'svelte'

  export interface RouteProps {
    path: string
    children?: any
  }

  export class Route extends SvelteComponent<RouteProps> {}

  interface TinroQuery {
    get(): Record<string, string>
    get(name: string): string | undefined
    set(name: string, value: string | number): void
    delete(name: string): void
    replace(query: Record<string, string>): void
    clear(): void
  }

  interface TinroLocation {
    query: TinroQuery
    hash: {
      get(): string
      set(value: string): void
      clear(): void
    }
  }

  export const router: {
    goto: (path: string, replace?: boolean) => void
    mode: {
      history: () => void
      hash: () => void
      memory: () => void
    }
    path: string
    location: TinroLocation
    subscribe: (
      callback: (route: {
        path: string
        url: string
        from?: string
        hash: string
        query: Record<string, string>
      }) => void
    ) => () => void
  }
}
