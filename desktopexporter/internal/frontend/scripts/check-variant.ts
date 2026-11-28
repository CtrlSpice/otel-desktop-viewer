import { spawnSync } from 'node:child_process'
import { mkdir, mkdtemp, rm } from 'node:fs/promises'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'

const frontend = fileURLToPath(new URL('../', import.meta.url))
const root = fileURLToPath(new URL('../../../../', import.meta.url))
const cache = join(frontend, 'node_modules', '.cache')
await mkdir(cache, { recursive: true })
const directory = await mkdtemp(join(cache, 'variant-'))
const fixture = join(directory, 'native.arrow')

function run(
  command: string,
  args: string[],
  cwd: string,
  env: NodeJS.ProcessEnv = process.env
): void {
  const result = spawnSync(command, args, { cwd, env, stdio: 'inherit' })
  if (result.error) throw result.error
  if (result.status !== 0) {
    throw new Error(`${command} failed with exit code ${result.status}`)
  }
}

try {
  run(
    'go',
    [
      'run',
      './desktopexporter/internal/store/arrowread/testdata/variantfixture',
      fixture,
    ],
    root
  )
  for (const mode of ['0', '1'] as const) {
    console.log(`Fresh native Variant compatibility: EVALS=${mode}`)
    run(
      process.execPath,
      [
        join(frontend, 'node_modules/vitest/vitest.mjs'),
        'run',
        'src/services/arrow/arrow-variant-compatibility.test.ts',
        '--maxWorkers=2',
      ],
      frontend,
      { ...process.env, EVALS: mode, VARIANT_ARROW_FIXTURE: fixture }
    )
  }
} finally {
  await rm(directory, { recursive: true, force: true })
}
