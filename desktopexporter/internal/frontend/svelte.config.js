import { vitePreprocess } from '@sveltejs/vite-plugin-svelte'

export default {
  // Svelte 5 strips TS types natively, but vitePreprocess runs scripts
  // through esbuild so non-erasable TS (e.g. enums) also works.
  preprocess: vitePreprocess(),
}
