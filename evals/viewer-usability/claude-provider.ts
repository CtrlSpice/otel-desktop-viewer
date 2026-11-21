import fs from 'node:fs'
import path from 'node:path'
import { homedir, userInfo } from 'node:os'
import { context as otelContext, propagation } from '@opentelemetry/api'
import { spawn, execFileSync, type ChildProcess } from 'node:child_process'
import { record, saveJson } from './runtime.ts'
import { stopOwnedGroup } from './run-cancellation.ts'
import { withBinaryTeardown, type RegisterShutdown } from './attempt-teardown.ts'
import type { PiContext } from './pi-provider.ts'

type ClaudeConfig = {
  root: string
  model: string
  binary?: string
  role?: 'investigator' | 'grader'
  cliBinary?: string
  configDir?: string
  source?: { evaluationId: string; resultID: string; repeatIndex: number; traceparent: string }
  telemetryEndpoint?: string
  timeoutMs?: number
}
const children = new Set<ChildProcess>()
export async function cancelClaudeAttempts() {
  await Promise.all([...children].map(child => stopOwnedGroup(child.pid!, child)))
}

export function claudeEvidence(events: Record<string, unknown>[]) {
  const calls = new Map<string, Record<string, unknown>>()
  const failures = new Set<string>()
  const models = new Set<string>()
  for (const event of events) {
    if (event.type !== 'assistant' && event.type !== 'user') continue
    const message = record(event.message)
    if (event.type === 'assistant' && typeof message.model === 'string' && message.model !== '<synthetic>') models.add(message.model)
    if (!Array.isArray(message.content)) continue
    for (const value of message.content) {
      const block = record(value)
      if (block.type === 'tool_use' && typeof block.id === 'string') calls.set(block.id, block)
      if (block.type === 'tool_result' && block.is_error && typeof block.tool_use_id === 'string') failures.add(block.tool_use_id)
    }
  }
  return { toolCalls: calls.size, failedToolCalls: failures.size, actualModels: [...models] }
}

export function claudeArguments(model: string, role: 'investigator' | 'grader', prompt: string) {
  return ['--model', model, '--effort', 'low', '--safe-mode', '--restricted',
    '--tools', role === 'grader' ? '' : 'Bash,Read,Edit,Write',
    ...(role === 'grader' ? [] : ['--allowedTools', 'Bash(./bin/otel-desktop-viewer *)', 'Read', 'Edit', 'Write']),
    '--permission-mode', 'dontAsk', '--strict-mcp-config', '--mcp-config', '{"mcpServers":{}}',
    '--no-chrome', '--no-session-persistence', '--max-turns', '80',
    '--output-format', 'stream-json', '--verbose', '-p', ...(role === 'grader' ? [] : [prompt])]
}

export default class ClaudeProvider {
  config: ClaudeConfig
  label?: string
  constructor({ config, label }: { config: ClaudeConfig; label?: string }) {
    this.config = config
    this.label = label
  }
  id() { return `claude-code:${this.config.model}` }

  async callApi(prompt: string, context: PiContext = {}) {
    const { telemetryEndpoint } = this.config
    const role = this.config.role ?? 'investigator'
    if (!['investigator', 'grader'].includes(role)) throw new Error('Unsupported Claude role')
    if (!telemetryEndpoint) throw new Error('Claude evaluation requires OTEL_EVAL_OBSERVER_HTTP for its separate telemetry receiver')
    const cli = this.config.cliBinary ?? 'claude'
    const version = execFileSync(cli, ['--version'], { encoding: 'utf8' }).trim()
    const runtimeVersion = role === 'grader' ? '2.1.326' : '2.1.307'
    if (version !== `${runtimeVersion} (Claude Code)`) throw new Error(`Use the verified unmodified Claude Code ${runtimeVersion} binary`)
    const source = this.config.source
    if (role === 'grader' && !/^00-[a-f0-9]{32}-[a-f0-9]{16}-0[01]$/.test((source ? source.traceparent : context.traceparent) ?? ''))
      throw new Error('Claude grader requires the investigation trace identity')
    if (this.config.configDir && !path.isAbsolute(this.config.configDir)) throw new Error('Claude configuration directory must be absolute')
    const root = fs.realpathSync(this.config.root)
    fs.mkdirSync(path.join(root, 'runs'), { recursive: true, mode: 0o700 })
    const evidence = fs.mkdtempSync(path.join(root, `runs/${role}-`))
    fs.chmodSync(evidence, 0o700)
    return withBinaryTeardown({ directory: evidence, canonicalBinary: this.config.binary, kind: 'attempt' },
      register => this.runAttempt(prompt, context, evidence, version, register))
  }

  async runAttempt(prompt: string, context: PiContext, evidence: string, version: string, register: RegisterShutdown) {
    const { model, telemetryEndpoint, source } = this.config
    const role = this.config.role ?? 'investigator'
    const runtimeVersion = role === 'grader' ? '2.1.326' : '2.1.307'
    const cli = this.config.cliBinary ?? 'claude'
    const workspace = path.join(evidence, 'workspace')
    fs.mkdirSync(workspace, { mode: 0o700 })
    fs.mkdirSync(path.join(evidence, 'tmp'), { mode: 0o700 })
    if (role === 'investigator') {
      if (!this.config.binary) throw new Error('Claude investigator requires the evaluated binary')
      fs.mkdirSync(path.join(workspace, 'bin'), { mode: 0o700 })
      fs.copyFileSync(this.config.binary, path.join(workspace, 'bin/otel-desktop-viewer'))
      fs.chmodSync(path.join(workspace, 'bin/otel-desktop-viewer'), 0o700)
    }
    fs.writeFileSync(path.join(evidence, 'prompt.txt'), prompt, { mode: 0o600, flag: 'wx' })
    const carrier: Record<string, string> = {}
    propagation.inject(otelContext.active(), carrier)
    const promptfoo = { evaluationId: context.evaluationId ?? source?.evaluationId, testIdx: context.testIdx,
      repeatIndex: context.repeatIndex ?? source?.repeatIndex,
      traceparent: carrier.traceparent ?? context.traceparent ?? source?.traceparent }
    const identity = { task: context.vars?.task_id, provider: this.id(), role, ...(source ? { source } : {}) }
    saveJson(path.join(evidence, 'identity.json'), { ...identity, promptfoo })
    // The official CLI owns its subscription login. No token is extracted or
    // transferred. Safe mode disables inherited instructions/plugins; restricted
    // mode confines file tools. Bash may invoke only the supplied viewer.
    const env = {
      HOME: homedir(), PATH: '/opt/homebrew/bin:/usr/bin:/bin', TMPDIR: path.join(evidence, 'tmp'), LANG: 'en_US.UTF-8',
      ...(this.config.configDir ? { CLAUDE_CONFIG_DIR: fs.realpathSync(this.config.configDir) } : {}),
      // macOS keychain lookup in the official CLI requires the login user.
      USER: userInfo().username, LOGNAME: userInfo().username,
      ...(promptfoo.traceparent ? { TRACEPARENT: promptfoo.traceparent } : {}),
      CLAUDE_CODE_ENABLE_TELEMETRY: '1', CLAUDE_CODE_ENHANCED_TELEMETRY_BETA: '1',
      OTEL_TRACES_EXPORTER: 'otlp', OTEL_LOGS_EXPORTER: 'otlp', OTEL_METRICS_EXPORTER: 'otlp',
      OTEL_EXPORTER_OTLP_PROTOCOL: 'http/json', OTEL_EXPORTER_OTLP_ENDPOINT: telemetryEndpoint,
      OTEL_TRACES_EXPORT_INTERVAL: '1000', OTEL_LOGS_EXPORT_INTERVAL: '1000', OTEL_METRIC_EXPORT_INTERVAL: '1000',
      OTEL_LOG_TOOL_DETAILS: '1', OTEL_LOG_TOOL_CONTENT: '1',
      OTEL_RESOURCE_ATTRIBUTES: `service.name=claude-code-eval,eval.task=${encodeURIComponent(String(context.vars?.task_id))}${context.repeatIndex === undefined ? '' : `,eval.rep=${context.repeatIndex}`}`,
    }
    const args = claudeArguments(model, role, prompt)
    saveJson(path.join(evidence, 'launch.json'), { cli, version, args, telemetryEndpoint,
      ...(role === 'grader' ? { inputTransport: 'stdin' } : {}) })
    const started = performance.now()
    const child = spawn(cli, args, { cwd: workspace, env, detached: true,
      stdio: role === 'grader' ? ['pipe', 'pipe', 'pipe'] : ['ignore', 'pipe', 'pipe'] })
    children.add(child)
    register(async () => {
      if (child.pid) await stopOwnedGroup(child.pid, child)
      children.delete(child)
      saveJson(path.join(evidence, 'owned-shutdown.json'), { pid: child.pid, exit: child.exitCode, signal: child.signalCode })
    })
    child.stdout!.on('data', data => fs.appendFileSync(path.join(evidence, 'events.jsonl'), data, { mode: 0o600 }))
    child.stderr!.on('data', data => fs.appendFileSync(path.join(evidence, 'stderr.log'), data, { mode: 0o600 }))
    let timedOut = false
    const timer = setTimeout(() => {
      timedOut = true
      if (child.pid) try { process.kill(-child.pid, 'SIGKILL') } catch {}
    }, this.config.timeoutMs ?? 600_000)
    let exit: number | null, inputError: string | undefined
    try {
      const closed = new Promise<number | null>((resolve, reject) => { child.once('error', reject); child.once('close', resolve) })
      if (role === 'grader') {
        child.stdin!.on('error', error => { inputError = error.message })
        child.stdin!.end(prompt)
      }
      exit = await closed
    }
    finally { clearTimeout(timer) }
    let events: Record<string, unknown>[] = []
    let malformed = false
    try { events = fs.readFileSync(path.join(evidence, 'events.jsonl'), 'utf8').split('\n').filter(line => line.trim()).map(line => record(JSON.parse(line))) }
    catch { malformed = true }
    const result = events.findLast(event => event.type === 'result')
    const init = events.find(event => event.type === 'system' && event.subtype === 'init')
    const stats = claudeEvidence(events)
    const sessionID = result?.session_id ?? init?.session_id
    const metadata = { ...identity, runtime: 'claude-code', runtimeVersion, model,
      evidence, workspace, ...(sessionID !== undefined ? { sessionID } : {}), exit,
      elapsedSeconds: (performance.now() - started) / 1000, ...stats,
      telemetryEndpoint, ...(result?.usage !== undefined ? { usage: result.usage } : {}),
      ...(result?.modelUsage !== undefined ? { modelUsage: result.modelUsage } : {}),
      ...(role === 'grader' ? { inputTransport: 'stdin', ...(inputError ? { inputError } : {}) } : {}) }
    const wrongModel = init?.model !== model || stats.actualModels.some(actual => actual !== model)
    const unexpectedTools = role === 'grader' && stats.toolCalls !== 0
    const response = exit !== 0 || timedOut || inputError || malformed || !result || result.is_error || typeof result.result !== 'string' || wrongModel || unexpectedTools
      ? { error: `Claude Code execution failed${wrongModel ? ' (model mismatch)' : ''}${unexpectedTools ? ' (grader used tools)' : ''}${inputError ? ' (prompt input failed)' : ''}${timedOut ? ' (timeout)' : ''}${result?.is_error && typeof result.result === 'string' ? ': ' + result.result : ''}; see ${evidence}`, metadata }
      : { output: result.result, metadata }
    if (response.output) fs.writeFileSync(path.join(evidence, 'answer.txt'), response.output, { mode: 0o600 })
    saveJson(path.join(evidence, 'response.json'), response)
    return response
  }
}
