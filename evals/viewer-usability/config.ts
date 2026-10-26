import path from 'node:path'
import cases from './tasks.json' with { type: 'json' }
import { readJson, record, runtimeRoot, SUITE } from './runtime.ts'
import { observerEnvironment } from './clean-environment.ts'

const root = runtimeRoot()
const observer = observerEnvironment()
Object.assign(process.env, observer)
const connection = record(readJson(path.join(root, 'connection.json')))
const authFile = process.env.OTEL_EVAL_PI_AUTH_FILE
if (!authFile || !path.isAbsolute(authFile))
  throw new Error('Set OTEL_EVAL_PI_AUTH_FILE to the private Pi auth.json file')

export default {
  description: 'Viewer usability',
  prompts: ['file://' + path.join(SUITE, 'prompt.txt')],
  tracing: { enabled: Boolean(observer.OTEL_EVAL_OBSERVER_HTTP) },
  providers: [...['gpt-6.1-sol', 'gpt-6-luna', 'gpt-5.6-luna'].map(model => ({
    id: 'file://' + path.join(SUITE, 'pi-provider.ts'),
    label: model,
    config: {
      root,
      provider: 'openai-codex',
      model,
      authFile,
      binary: connection.binary,
      role: 'investigator',
      timeoutMs: 600_000,
    },
  })), {
    id: 'file://' + path.join(SUITE, 'claude-provider.ts'),
    label: 'claude-opus-5-5',
    config: {
      root,
      model: 'claude-opus-5-5',
      binary: connection.binary,
      cliBinary: process.env.OTEL_EVAL_CLAUDE_BINARY,
      telemetryEndpoint: process.env.OTEL_EVAL_OBSERVER_HTTP,
      timeoutMs: 600_000,
    },
  }],
  evaluateOptions: { repeat: 3, maxConcurrency: 1, cache: false },
  defaultTest: {
    vars: { ...connection, binary: './bin/otel-desktop-viewer' },
    options: {
      provider: {
        id: 'file://' + path.join(SUITE, 'pi-provider.ts'),
        config: { root, role: 'grader', provider: 'openai-codex', model: 'gpt-6.1-sol', authFile, timeoutMs: 180_000 },
      },
    },
    assert: [{ type: 'llm-rubric', threshold: 1, value: 'file://' + path.join(SUITE, 'grading-evidence.ts') }],
  },
  tests: cases.map(task => ({
    description: task.id,
    vars: { task_id: task.id, goal: task.goal },
    ...(task.id === 'trace-export' ? { assert: [{ type: 'javascript', metric: 'export-file', value: 'file://' + path.join(SUITE, 'grading-evidence.ts') + ':verifyArtifact' }] } : {}),
  })),
  outputPath: [path.join(root, 'promptfoo.json'), path.join(root, 'promptfoo-rows.jsonl')],
  writeLatestResults: true,
}
