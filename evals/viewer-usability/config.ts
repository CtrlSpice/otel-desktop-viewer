import fs from 'node:fs'
import path from 'node:path'
import cases from './tasks.json' with { type: 'json' }
import { readJson, record, runtimeRoot, SUITE } from './runtime.ts'
const connection = record(readJson(path.join(runtimeRoot(), 'connection.json')))
// No cohort permission policy or authentication route is inferred from personal config.
const isolationFile = process.env.OTEL_EVAL_ISOLATION_FILE
let isolation: Record<string, unknown> | undefined
if (isolationFile) {
  if (
    !path.isAbsolute(isolationFile) ||
    fs.statSync(isolationFile).mode & 0o077
  ) {
    throw new Error(
      'OTEL_EVAL_ISOLATION_FILE must reference a private absolute JSON file'
    )
  }
  try {
    isolation = record(readJson(isolationFile))
  } catch {
    throw new Error('Cannot parse OTEL_EVAL_ISOLATION_FILE')
  }
  if (isolation.authEnvironment)
    throw new Error('Use authEnvironmentFile rather than inline credentials')
}
const models = [
  'openai/gpt-6.1-sol',
  'openai/gpt-5.6-sol',
  'openai/gpt-5.6-luna',
  'openai/gpt-5.6-terra',
]
export default {
  description:
    'Viewer usability pilot: natural command choice, six tasks, four available models',
  prompts: ['file://' + path.join(SUITE, 'prompt.txt')],
  providers: models.map(model => ({
    id: 'file://' + path.join(SUITE, 'provider.ts'),
    label: model,
    config: { model, isolation },
  })),
  evaluateOptions: { maxConcurrency: 1 },
  defaultTest: {
    vars: connection,
    assert: [
      { type: 'javascript', value: 'file://' + path.join(SUITE, 'grade.ts') },
    ],
  },
  tests: cases.map(c => ({
    description: c.id,
    vars: { task_id: c.id, goal: c.goal },
  })),
}
