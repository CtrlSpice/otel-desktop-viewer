/// <reference types="vitest/config" />
import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import { svelteTesting } from '@testing-library/svelte/vite'
import tailwindcss from '@tailwindcss/vite'
import { readdirSync, readFileSync } from 'node:fs'
import { join, relative } from 'node:path'
import { fileURLToPath, URL } from 'node:url'

const evals = process.env.EVALS ?? '0'
if (evals !== '0' && evals !== '1') throw new Error('EVALS must be 0 or 1')
const evalsBuilt = evals === '1'
const useTailwind = !process.env.VITEST || process.env.VITEST_BROWSER === '1'

const JSDOM_ENV_RE = /@vitest-environment\s+jsdom\b/
const ISOLATED_NODE_TEST_RE =
  /\bvi\s*\.\s*(?:mock|doMock|unmock|hoisted|resetModules|stubGlobal|unstubAllGlobals|stubEnv|unstubAllEnvs|useFakeTimers|spyOn|restoreAllMocks|clearAllMocks|resetAllMocks)\b|\bprocess\.env\b|@\/test\/(?:telemetry-service-fixtures|service-map-fixtures)\b/

interface TestFilePartition {
  domFiles: string[]
  isolatedUnitFiles: string[]
}

function partitionTestFiles(frontendRoot: string): TestFilePartition {
  const domFiles: string[] = []
  const isolatedUnitFiles: string[] = []

  function walk(dir: string): void {
    for (const entry of readdirSync(dir, { withFileTypes: true })) {
      const fullPath = join(dir, entry.name)
      if (entry.isDirectory()) {
        walk(fullPath)
        continue
      }
      if (!entry.name.endsWith('.test.ts')) continue
      const source = readFileSync(fullPath, 'utf8')
      const relPath = relative(frontendRoot, fullPath).replaceAll('\\', '/')
      if (JSDOM_ENV_RE.test(source)) {
        domFiles.push(relPath)
      } else if (ISOLATED_NODE_TEST_RE.test(source)) {
        isolatedUnitFiles.push(relPath)
      }
    }
  }

  walk(join(frontendRoot, 'src'))
  domFiles.sort()
  isolatedUnitFiles.sort()
  return { domFiles, isolatedUnitFiles }
}

const frontendRoot = fileURLToPath(new URL('.', import.meta.url))
const testProjects = process.env.VITEST
  ? (() => {
      const { domFiles, isolatedUnitFiles } = partitionTestFiles(frontendRoot)
      return [
        {
          extends: true,
          test: {
            name: 'unit',
            environment: 'node',
            isolate: false,
            setupFiles: [],
            include: ['src/**/*.test.ts'],
            exclude: [...domFiles, ...isolatedUnitFiles],
          },
        },
        {
          extends: true,
          test: {
            name: 'unit-isolated',
            environment: 'node',
            isolate: true,
            setupFiles: [],
            include: isolatedUnitFiles,
          },
        },
        {
          extends: true,
          test: {
            name: 'dom',
            environment: 'jsdom',
            setupFiles: [
              './src/test/setup.ts',
              '@testing-library/svelte/vitest',
            ],
            include: domFiles,
          },
        },
      ]
    })()
  : undefined

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [
    ...(useTailwind ? [tailwindcss()] : []),
    svelte(),
    svelteTesting({ autoCleanup: false }),
    {
      name: 'build-features',
      generateBundle(_options, bundle) {
        if (!evalsBuilt) {
          for (const output of Object.values(bundle)) {
            if (output.type !== 'chunk') continue
            for (const [id, module] of Object.entries(output.modules)) {
              if (module.renderedLength === 0) continue
              if (
                /\/src\/(features\/evals\/|services\/files\/import-file\.ts)/.test(
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
      '/api/changes': {
        target: process.env.VIEWER_PROXY_TARGET || 'http://localhost:8000',
        changeOrigin: true,
      },
      '/api/import-target': {
        target: process.env.VIEWER_PROXY_TARGET || 'http://localhost:8000',
        changeOrigin: true,
      },
      '/api/data': {
        target: process.env.VIEWER_PROXY_TARGET || 'http://localhost:8000',
        changeOrigin: true,
      },
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
    css: false,
    projects: testProjects,
  },
})
