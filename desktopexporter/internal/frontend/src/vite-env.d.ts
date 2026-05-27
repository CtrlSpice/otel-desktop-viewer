/// <reference types="svelte" />
/// <reference types="vite/client" />
/// <reference types="d3-interpolate" />
/// <reference types="d3-scale-chromatic" />

declare module '*.svelte' {
  import type { ComponentType } from 'svelte'
  const component: ComponentType
  export default component
}
