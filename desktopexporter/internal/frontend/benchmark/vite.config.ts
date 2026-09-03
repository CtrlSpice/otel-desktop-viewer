import { fileURLToPath, URL } from 'node:url'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import tailwindcss from '@tailwindcss/vite'
import { defineConfig } from 'vite'

const benchmarkEntry = fileURLToPath(new URL('./index.html', import.meta.url))
const benchmarkOutput = fileURLToPath(
  new URL('../dist-benchmark', import.meta.url)
)
const productionSource = fileURLToPath(new URL('../src', import.meta.url))
const svelteConfig = fileURLToPath(
  new URL('../svelte.config.js', import.meta.url)
)

export default defineConfig({
  plugins: [tailwindcss(), svelte({ configFile: svelteConfig })],
  resolve: {
    alias: {
      '@': productionSource,
    },
  },
  publicDir: false,
  build: {
    outDir: benchmarkOutput,
    emptyOutDir: true,
    chunkSizeWarningLimit: 1200,
    rollupOptions: {
      input: benchmarkEntry,
    },
  },
  preview: {
    host: '127.0.0.1',
    port: 4174,
    strictPort: true,
    proxy: {
      '/benchmark-api': {
        target: 'http://127.0.0.1:8002',
        changeOrigin: true,
      },
      '/rpc': {
        target: 'http://127.0.0.1:8001',
        changeOrigin: true,
      },
    },
  },
})
