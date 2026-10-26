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
  'openai/gpt-6-luna',
  'openai/gpt-5.6-luna',
]
export default {
  description:
    'Viewer usability: eleven natural investigations, separate provisional Sol6.1 grading',
  prompts: ['file://' + path.join(SUITE, 'prompt.txt')],
  // beforeEach replaces this inert provider with a fresh native attempt.
  providers: ['echo'],
  extensions: ['beforeEach', 'afterEach', 'afterAll'].map(hook => 'file://' + path.join(SUITE, 'native-hooks.mjs') + ':' + hook),
  evaluateOptions: { maxConcurrency: 1 },
  defaultTest: {
    vars: connection,
    assert: [
      { type: 'javascript', value: 'file://' + path.join(SUITE, 'grade.ts') },
    ],
  },
  tests: cases.flatMap(c => models.map(model => ({
    description: c.id,
    vars: { task_id: c.id, goal: c.goal, eval_model: model },
  }))),
}
