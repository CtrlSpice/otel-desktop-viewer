import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vitest/config'

const frontendRoot = fileURLToPath(new URL('..', import.meta.url))

export default defineConfig({
  root: frontendRoot,
  test: {
    environment: 'node',
    include: ['benchmark/**/*.test.ts'],
  },
})
