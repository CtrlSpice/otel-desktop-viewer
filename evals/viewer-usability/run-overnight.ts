import fs from 'node:fs'
import path from 'node:path'
import { createHash } from 'node:crypto'
import { prepare } from './prepare.ts'
import { verify } from './verify.ts'
import { runPilot } from './run-pilot.ts'
import { datasetSource, stageDataset } from './stage-dataset.ts'
import { runtimeRoot, saveJson } from './runtime.ts'
import cases from './tasks.json' with { type: 'json' }
import { GRADER_MODEL } from './grade.ts'

const root = runtimeRoot()
const binary = process.env.OTEL_EVAL_VIEWER_BINARY
if (!binary || !path.isAbsolute(binary))
  throw new Error(
    'Set absolute OTEL_EVAL_VIEWER_BINARY for the owned overnight viewer'
  )
const source = datasetSource()
const models = ['openai/gpt-6-luna', 'openai/gpt-6.1-sol', 'openai/gpt-6-astra']
const repetitions = 3
if (!process.env.OTEL_EVAL_ISOLATION_FILE)
  throw new Error('Set OTEL_EVAL_ISOLATION_FILE before running models')

saveJson(path.join(root, 'run-manifest.json'), {
  startedAt: new Date().toISOString(),
  models,
  graderModel: GRADER_MODEL,
  gradingProvisional: true,
  tasks: cases.length,
  repetitions,
  investigatorSessions: cases.length * models.length * repetitions,
  timeoutMs: 600000,
  dataset: source,
  binary,
  binarySHA256: createHash('sha256')
    .update(fs.readFileSync(binary))
    .digest('hex'),
  viewerSourceCommit: process.env.OTEL_EVAL_VIEWER_COMMIT,
  productMainCommit: process.env.OTEL_EVAL_PRODUCT_MAIN_COMMIT,
  evalSourceCommit: process.env.OTEL_EVAL_SOURCE_COMMIT,
  isolationSettingsFile: process.env.OTEL_EVAL_ISOLATION_FILE,
})
fs.copyFileSync(
  binary,
  path.join(root, 'otel-desktop-viewer'),
  fs.constants.COPYFILE_EXCL
)
stageDataset(source, path.join(root, 'fixture'))
const ready = await prepare(root)
let phase = 'verification'
try {
  await verify('preflight', root)
  phase = 'evaluation'
  console.log(
    JSON.stringify({
      phase,
      root,
      viewerPID: ready.pid,
      endpoint: ready.endpoint,
    })
  )
  const exit = await runPilot(root)
  saveJson(path.join(root, 'overnight-completion.json'), {
    finishedAt: new Date().toISOString(),
    exit,
  })
  process.exitCode = exit ?? 1
} catch (error) {
  saveJson(path.join(root, 'overnight-failure.json'), {
    phase,
    error: error instanceof Error ? error.message : String(error),
  })
  process.exitCode = 1
} finally {
  ready.process.kill('SIGTERM')
  const timer = setTimeout(() => ready.process.kill('SIGKILL'), 30000)
  try {
    const exit = await ready.closed
    saveJson(path.join(root, 'owned-shutdown.json'), {
      pid: ready.pid,
      waited: true,
      exit,
    })
  } finally {
    clearTimeout(timer)
  }
}
