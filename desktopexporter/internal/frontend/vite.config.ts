/// <reference types="vitest/config" />
import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import { svelteTesting } from '@testing-library/svelte/vite'
import tailwindcss from '@tailwindcss/vite'
import { fileURLToPath, URL } from 'node:url'

const evals = process.env.EVALS ?? '0'
if (evals !== '0' && evals !== '1') throw new Error('EVALS must be 0 or 1')
const evalsBuilt = evals === '1'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [
    tailwindcss(),
    svelte(),
    svelteTesting(),
    {
      name: 'build-features',
      generateBundle(_options, bundle) {
        if (!evalsBuilt) {
          for (const output of Object.values(bundle)) {
            if (output.type !== 'chunk') continue
            for (const [id, module] of Object.entries(output.modules)) {
              if (module.renderedLength === 0) continue
              if (
                /\/src\/(components\/evals\/|pages\/Evals|services\/(eval-|import-file\.ts))/.test(
                  id.replaceAll('\\', '/')
                )
              )
                this.error(`Public build contains an Evals module: ${id}`)
            }
          }
        }
        this.emitFile({
          type: 'asset',
          fileName: 'build-features.json',
          source: JSON.stringify({ evals: evalsBuilt }) + '\n',
        })
      },
    },
  ],
  define: { __EVALS_BUILT__: evalsBuilt },
  cacheDir:
    process.env.VIEWER_VITE_CACHE_DIR ||
    `node_modules/.vite${evalsBuilt ? '-evals' : ''}`,
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  server: {
    port: 3001, // Fixed port for v2 frontend
    strictPort: true, // Fail if port is in use instead of trying another
    proxy: {
      '/rpc': {
        target: process.env.VIEWER_PROXY_TARGET || 'http://localhost:8000',
        changeOrigin: true,
      },
      '/export': {
        target: process.env.VIEWER_PROXY_TARGET || 'http://localhost:8000',
        changeOrigin: true,
      },
      '/api/features': {
        target: process.env.VIEWER_PROXY_TARGET || 'http://localhost:8000',
        changeOrigin: true,
      },
      '/api/evals': {
        target: process.env.VIEWER_PROXY_TARGET || 'http://localhost:8000',
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: evalsBuilt ? 'dist-evals' : 'dist',
    emptyOutDir: true,
    // This app is served from localhost out of a Go-embedded binary, so
    // transfer size is irrelevant. The experimental Evals workspace is lazy
    // loaded so disabled experiments do not load their presentation or examples.
    // Vite's default 500 kB warning assumes a public web app; keep the
    // warning as a canary for genuine bloat instead.
    chunkSizeWarningLimit: 1200,
  },
  test: {
    include: ['src/**/*.test.ts'],
    setupFiles: ['./src/test/setup.ts'],
  },
})
