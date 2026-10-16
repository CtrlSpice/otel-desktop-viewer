import fs from 'node:fs'
import path from 'node:path'
import { spawn, execFile } from 'node:child_process'
import { once } from 'node:events'
import { promisify } from 'node:util'
import { pathToFileURL } from 'node:url'
import { context as otelContext } from '@opentelemetry/api'
import { createCleanEnvironment, credentialEnvironment, observerEnvironment, SUPPORTED_OPENCODE_VERSION } from './clean-environment.ts'
import { isolationSettings, sessionSummary } from './provider.ts'
import { permissionDenial, viewerClientCall } from './permission-stop.ts'
import { readJson, record, runtimeRoot } from './runtime.ts'
import { AttemptTelemetry, withOpenCodeContext } from './eval-telemetry.ts'
import { modelExecutionErrors, recordExecutionError } from './execution-errors.ts'

const attempts = new Map<string, NativeAttempt>()
// Import the public package at runtime; its declarations require optional database drivers.
const promptfooPackage = 'promptfoo'
const { loadApiProvider } = await import(promptfooPackage)
type CallApiContextParams = { traceparent?: string; vars?: Record<string, unknown>; prompt?: { config?: Record<string, unknown> } }
type ProviderResponse = { output?: unknown; error?: string; sessionId?: string; metadata?: Record<string, unknown>; tokenUsage?: unknown }
type ApiProvider = { callApi(prompt: string, context?: CallApiContextParams, options?: { abortSignal: AbortSignal }): Promise<ProviderResponse> }
type TestCase = { vars?: Record<string, unknown>; metadata?: Record<string, unknown>; provider?: unknown }

export class NativeAttempt {
  label: string
  config: Record<string, unknown>
  model: string
  role: 'investigator' | 'grader'
  task: unknown
  isolated: ReturnType<typeof createCleanEnvironment>
  evidence: string
  child?: ReturnType<typeof spawn>
  baseUrl = ''
  native?: ApiProvider
  binary: string
  redact: (text: string) => string
  started: number
  timeoutMs: number
  diagnostics = () => {}
  telemetry?: AttemptTelemetry
  telemetryOutcome?: { error?: string; output?: unknown }
  private closing?: Promise<void>
  private constructor(
    model: string,
    role: 'investigator' | 'grader',
    task: unknown,
    isolated: ReturnType<typeof createCleanEnvironment>,
    evidence: string,
    binary: string,
    redact: (text: string) => string,
    started: number,
    timeoutMs: number,
    config: Record<string, unknown>,
  ) {
    this.label = model
    this.config = config
    this.model = model
    this.role = role
    this.task = task
    this.isolated = isolated
    this.evidence = evidence
    this.binary = binary
    this.redact = redact
    this.started = started
    this.timeoutMs = timeoutMs
  }
  id() { return 'opencode:sdk:' + this.model }
  save(name: string, value: unknown) {
    fs.writeFileSync(path.join(this.evidence, name), this.redact(typeof value === 'string' ? value : JSON.stringify(value, null, 2)), { mode: 0o600, flag: 'wx' })
  }
  static async create(model: string, role: 'investigator' | 'grader', task: unknown, isolation: unknown, copy = true, timeoutMs = 600000) {
    const root = runtimeRoot()
    if (fs.existsSync(path.join(root, 'harness-blocker.json'))) throw new Error('Evaluation stopped by viewer permission denial')
    if (fs.existsSync(path.join(root, 'execution-error.json'))) throw new Error('Evaluation stopped by an execution error')
    if (role === 'investigator' && modelExecutionErrors(root).some(error => error.model === model)) throw new Error('Investigator model paused after an execution error')
    const started = performance.now()
    const settings = isolationSettings(isolation)
    if (settings.authEnvironment) throw new Error('Use credential file references')
    for (const file of [settings.authFile, settings.authEnvironmentFile])
      if (file && (!path.isAbsolute(file) || fs.statSync(file).mode & 0o077)) throw new Error('Private absolute credential references required')
    const authEnvironment = settings.authEnvironmentFile ? credentialEnvironment(readJson(settings.authEnvironmentFile)) : {}
    const secrets = Object.values(authEnvironment)
    if (settings.authFile) {
      const auth = record(readJson(settings.authFile))
      const collect = (value: unknown) => {
        if (typeof value === 'string' && value.length) secrets.push(value)
        else if (value && typeof value === 'object') for (const [key, item] of Object.entries(value)) if (key !== 'type') collect(item)
      }
      for (const id of new Set([model.split('/')[0], ...settings.providerIDs ?? []])) collect(auth[id])
    }
    secrets.sort((a, b) => b.length - a.length)
    const redact = (text: string) => { for (const secret of secrets) if (secret) text = text.split(secret).join('[REDACTED]'); return text }
    for (const name of ['workspaces', 'runs']) fs.mkdirSync(path.join(root, name), { recursive: true, mode: 0o700 })
    const evidence = fs.mkdtempSync(path.join(root, 'runs', `${role}-${model}-${task}-`.replace(/[^a-zA-Z0-9_.-]/g, '-')))
    fs.chmodSync(evidence, 0o700)
    let isolated: ReturnType<typeof createCleanEnvironment>
    try {
      isolated = createCleanEnvironment({ parent: path.join(root, 'workspaces'), model, permission: settings.permission, authFile: settings.authFile, authEnvironment, modelCatalogueFile: settings.modelCatalogueFile, providerIDs: settings.providerIDs, executablePaths: settings.executablePaths })
    } catch {
      fs.writeFileSync(path.join(evidence, 'failure.json'), JSON.stringify({ role, model, task, evidence, phase: 'private profile', error: 'Native private profile setup failed' }), { mode: 0o600, flag: 'wx' })
      throw new Error('Native private profile setup failed; see ' + evidence)
    }
    const permission = record(settings.permission)
    const external = permission.external_directory
    permission.external_directory = { ...(typeof external === 'string' ? { '*': external } : record(external ?? {})), [path.join(isolated.env.XDG_DATA_HOME, 'opencode/tool-output/*')]: 'allow' }
    fs.writeFileSync(path.join(isolated.env.OPENCODE_CONFIG_DIR, 'opencode.json'), JSON.stringify({ ...isolated.config, permission }), { mode: 0o600 })
    if (role === 'investigator') {
      fs.mkdirSync(path.join(isolated.workspace, 'bin'), { mode: 0o700 })
      fs.copyFileSync(path.join(root, 'otel-desktop-viewer'), path.join(isolated.workspace, 'bin/otel-desktop-viewer'))
      fs.chmodSync(path.join(isolated.workspace, 'bin/otel-desktop-viewer'), 0o700)
    }
    const binary = settings.opencodeBinary ?? 'opencode'
    const check = async (args: string[]) => promisify(execFile)(binary, args, { env: isolated.env, cwd: isolated.workspace, timeout: Math.max(1, Math.floor(timeoutMs - (performance.now() - started))), maxBuffer: Infinity })
    let phase = 'version'
    try {
      const checked = await check(['--version'])
      fs.writeFileSync(path.join(evidence, 'version.stdout'), redact(checked.stdout), { mode: 0o600, flag: 'wx' })
      fs.writeFileSync(path.join(evidence, 'version.stderr'), redact(checked.stderr), { mode: 0o600, flag: 'wx' })
      const version = checked.stdout.trim()
      if (version !== SUPPORTED_OPENCODE_VERSION) throw new Error('Unsupported OpenCode version')
      phase = 'model catalogue'
      let catalogue = (await check(['models', model.split('/')[0]])).stdout
      if (!catalogue.trim().split(/\r?\n/).includes(model)) catalogue = (await check(['models', model.split('/')[0], '--refresh'])).stdout
      if (!catalogue.trim().split(/\r?\n/).includes(model)) throw new Error('Requested model absent from clean catalogue')
      phase = 'authentication'
      const auth = path.join(isolated.env.XDG_DATA_HOME, 'opencode/auth.json')
      const environmentAuth = model.startsWith('openai/') ? !!authEnvironment.OPENAI_API_KEY : Object.keys(authEnvironment).length > 0
      if (!environmentAuth && (!fs.existsSync(auth) || !record(readJson(auth))[model.split('/')[0]])) throw new Error('Selected model authentication missing')
      const [provider_id, ...parts] = model.split('/')
      const config = { provider_id, model: parts.join('/'), working_dir: isolated.workspace, copy_working_dir: copy ? 'copy' : false, persist_sessions: true, agent: 'build', permission }
      const attempt = new NativeAttempt(model, role, task, isolated, evidence, binary, redact, started, timeoutMs, config)
      attempt.save('launch.json', { model, role, task, binary, version, isolationRoot: isolated.root, timeoutMs })
      attempts.set(evidence, attempt)
      return attempt
    } catch (error) {
      fs.writeFileSync(path.join(evidence, 'failure.json'), JSON.stringify({ role, model, task, evidence, isolationRoot: isolated.root, phase, error: 'Native isolation/authentication/model setup failed', diagnostic: phase === 'version' && error instanceof Error ? redact(error.message) : undefined }), { mode: 0o600, flag: 'wx' })
      throw new Error('Native isolation/authentication/model setup failed; see ' + evidence)
    }
  }
  private async start(workspace: string) {
    let phase = 'workspace binding'
    try {
      if (!path.isAbsolute(workspace) || !fs.statSync(workspace).isDirectory()) throw new Error('Assigned absolute workspace required')
      if (this.config.copy_working_dir && workspace === this.isolated.workspace) throw new Error('Framework workspace copy required')
      this.isolated.env.TMPDIR = path.join(workspace, 'tmp')
      fs.mkdirSync(this.isolated.env.TMPDIR, { mode: 0o700 })
      this.save('runtime.json', { workspace, tmpdir: this.isolated.env.TMPDIR })
      phase = 'server startup'
      const child = this.child = spawn(this.binary, ['serve', '--hostname', '127.0.0.1', '--port', '0'], { env: this.isolated.env, cwd: workspace, detached: process.platform !== 'win32', stdio: ['ignore', 'pipe', 'pipe'] })
      if (child.pid) this.save('process.pid', String(child.pid))
      let stdout = '', stderr = ''
      child.stdout!.on('data', chunk => { stdout += chunk })
      child.stderr!.on('data', chunk => { stderr += chunk })
      this.diagnostics = () => {
        for (const [name, text] of [['server.stdout.log', stdout], ['server.stderr.log', stderr]]) fs.writeFileSync(path.join(this.evidence, name), this.redact(text), { mode: 0o600 })
      }
      child.once('close', this.diagnostics)
      const closed = once(child, 'close')
      const baseUrl = await new Promise<string>((resolve, reject) => {
        const timer = setTimeout(() => reject(new Error('Owned server startup timed out')), Math.min(30000, Math.max(1, this.timeoutMs - (performance.now() - this.started))))
        const inspect = () => { const url = stdout.match(/http:\/\/127\.0\.0\.1:\d+/)?.[0]; if (url) { clearTimeout(timer); resolve(url) } }
        child!.stdout!.on('data', inspect)
        child!.once('error', error => { clearTimeout(timer); reject(error) })
        closed.then(() => { clearTimeout(timer); reject(new Error('Owned server exited during startup')) }, reject)
      })
      this.baseUrl = baseUrl
      phase = 'native provider loading'
      this.native = await loadApiProvider('opencode:sdk', { options: { config: { ...this.config, baseUrl, working_dir: workspace } } })
    } catch {
      if (this.child) await stopOwned(this.child)
      this.save('failure.json', { role: this.role, model: this.model, task: this.task, evidence: this.evidence, isolationRoot: this.isolated.root, phase, error: 'Native workspace/runtime setup failed' })
      throw new Error('Native workspace/runtime setup failed; see ' + this.evidence)
    }
  }
  async callApi(prompt: string, context?: CallApiContextParams): Promise<ProviderResponse> {
    const workspace = String(context?.prompt?.config?.working_dir ?? this.config.working_dir)
    const controller = new AbortController()
    let timedOut = false
    let blocked = false
    let sessionID: string | undefined
    const latestParts = new Map<string, Record<string, unknown>>()
    const streamController = new AbortController()
    const api = async (route: string, method = 'GET') => {
      const response = await fetch(this.baseUrl + route + (route.includes('?') ? '&' : '?') + 'directory=' + encodeURIComponent(workspace), { method, signal: AbortSignal.timeout(30000) })
      if (!response.ok) throw new Error('Owned server evidence request failed')
      return response.text()
    }
    const stop = async () => { controller.abort(); if (sessionID) await api('/session/' + sessionID + '/abort', 'POST') }
    const timer = setTimeout(() => { timedOut = true; void abortOwned().catch(() => this.close()) }, Math.max(1, this.timeoutMs - (performance.now() - this.started)))
    let monitor: Promise<void> | undefined
    let monitorError = false
    let invoked = false
    let escalation: ReturnType<typeof setTimeout> | undefined
    const abortOwned = () => {
      if (!escalation) escalation = setTimeout(() => {
        try { if (this.child?.pid) process.kill(process.platform === 'win32' ? this.child.pid : -this.child.pid, 'SIGKILL') }
        catch (error) { if (!(error instanceof Error && 'code' in error && error.code === 'ESRCH')) throw error }
      }, 30000)
      return stop()
    }
    try {
      if (performance.now() - this.started >= this.timeoutMs) { timedOut = true; throw new Error('Attempt deadline exceeded before startup') }
      const observer = observerEnvironment()
      if (observer.OTEL_EVAL_OBSERVER_HTTP) {
        this.telemetry = await new AttemptTelemetry({ root: runtimeRoot(), evidence: this.evidence,
          model: this.model, role: this.role, task: String(this.task), parent: context?.traceparent,
          redact: this.redact }).listen()
        const configFile = path.join(this.isolated.env.OPENCODE_CONFIG_DIR, 'opencode.json')
        const config = record(readJson(configFile))
        fs.writeFileSync(configFile, JSON.stringify({ ...config,
          plugin: [pathToFileURL(path.join(import.meta.dirname, 'native-telemetry-plugin.mjs')).href],
          experimental: { ...record(config.experimental ?? {}), openTelemetry: true },
        }), { mode: 0o600 })
        Object.assign(this.isolated.env, {
          // Project/global discovery is still isolated. Enable only the explicit
          // trusted adapter; --pure would suppress it along with external plugins.
          OPENCODE_PURE: '0',
          OTEL_EXPORTER_OTLP_ENDPOINT: observer.OTEL_EVAL_OBSERVER_HTTP,
          OTEL_BSP_SCHEDULE_DELAY: '100', OTEL_BSP_MAX_QUEUE_SIZE: '65536',
          OTEL_EVAL_HOOK_ENDPOINT: this.telemetry.endpoint, OTEL_EVAL_HOOK_TOKEN: this.telemetry.token,
          OTEL_RESOURCE_ATTRIBUTES: Object.entries({ 'eval.run.id': path.basename(runtimeRoot()),
            'eval.attempt.id': path.basename(this.evidence), 'eval.task.id': String(this.task),
            'eval.role': this.role }).map(([key, value]) => key + '=' + encodeURIComponent(value)).join(','),
        })
      }
      await this.start(workspace)
      if (timedOut || controller.signal.aborted) throw new Error('Attempt deadline exceeded during startup')
      const stream = await fetch(this.baseUrl + '/event?directory=' + encodeURIComponent(workspace), { signal: streamController.signal })
      if (!stream.ok || !stream.body) throw new Error('Owned event stream unavailable')
      monitor = (async () => {
        let pending = ''
        for await (const chunk of stream.body!.pipeThrough(new TextDecoderStream())) {
          pending += chunk
          let end: number
          while ((end = pending.indexOf('\n\n')) >= 0) {
            const frame = pending.slice(0, end); pending = pending.slice(end + 2)
            for (const line of frame.split('\n')) {
              if (!line.startsWith('data: ')) continue
              const raw = line.slice(6)
              fs.appendFileSync(path.join(this.evidence, 'native-events.jsonl'), this.redact(raw) + '\n', { mode: 0o600 })
              const event = record(JSON.parse(raw))
              const properties = record(event.properties ?? {})
               if (event.type === 'session.created') {
                 const id = record(properties.info).id
                 if (typeof id !== 'string' || (sessionID && sessionID !== id)) throw new Error('Unexpected native session identity')
                 sessionID = id
               }
              let denial: ReturnType<typeof permissionDenial>
              if (event.type === 'permission.asked' && properties.sessionID === sessionID) {
                const input = record(properties.metadata ?? {})
                const tool = record(properties.tool ?? {})
                denial = { sessionID: sessionID!, callID: String(tool.callID ?? properties.id), tool: String(properties.permission), input, reason: 'Native permission request is pending', stopsRun: properties.permission === 'bash' && typeof input.command === 'string' && viewerClientCall(input.command, workspace, [path.join(workspace, 'bin/otel-desktop-viewer')]), rawEvent: raw }
              } else if (event.type === 'message.part.updated') {
                const part = record(properties.part)
                if (part.sessionID !== sessionID) continue
                if (typeof part.id === 'string') latestParts.set(part.id, part)
                denial = permissionDenial(JSON.stringify({ type: 'tool_use', sessionID, part }), workspace, [path.join(workspace, 'bin/otel-desktop-viewer')])
              }
              if (!denial) continue
              fs.appendFileSync(path.join(this.evidence, 'permission-denials.jsonl'), this.redact(JSON.stringify(denial)) + '\n', { mode: 0o600 })
              if (!denial.stopsRun || this.role !== 'investigator' || blocked) continue
              blocked = true
              this.save('failure.json', { status: 'harness-blocker', role: this.role, model: this.model, task: this.task, evidence: this.evidence, sessionID, denial })
                fs.writeFileSync(path.join(runtimeRoot(), 'harness-blocker.json'), this.redact(JSON.stringify({ status: 'harness-blocker', role: this.role, model: this.model, task: this.task, evidence: this.evidence, isolationRoot: this.isolated.root, workspace, runnerPID: process.pid, sessionPID: this.child?.pid, sessionID, denial })), { flag: 'wx', mode: 0o600 })
              await abortOwned()
            }
          }
        }
      })().catch(async () => { if (!streamController.signal.aborted) { monitorError = true; await abortOwned().catch(() => {}) } })
      if (this.redact(prompt) !== prompt) throw new Error('Prompt contains selected credential material')
      this.save('prompt.txt', prompt)
      this.telemetry?.span.setAttribute('gen_ai.input', prompt)
      const invoke = () => { invoked = true; return this.native!.callApi(prompt, context, { abortSignal: controller.signal }) }
      const response = this.telemetry
        ? await otelContext.with(this.telemetry.context, () => withOpenCodeContext(this.baseUrl, this.telemetry!.context, invoke))
        : await invoke()
      if (monitorError) throw new Error('Native denial monitor failed')
      if (typeof response.sessionId === 'string') {
        if (sessionID && response.sessionId !== sessionID) throw new Error('Cross-session native response rejected')
        sessionID = response.sessionId
      }
      if (!sessionID) throw new Error('Missing native session identity')
      const historyText = await api('/session/' + sessionID + '/message')
      this.save('native-history.json', historyText)
      const history = JSON.parse(historyText)
      if (!Array.isArray(history)) throw new Error('Malformed native history')
      const final = history.filter(message => message.info?.role === 'assistant').at(-1)
      let incomplete = final?.info?.error !== undefined || final?.info?.finish !== 'stop'
      const events: string[] = []
      const partIDs = new Set<string>()
      for (const message of history) {
        if (message.info?.sessionID !== sessionID) throw new Error('Cross-session native history rejected')
        if (message.info?.role === 'assistant' && (message.info.modelID !== this.model.slice(this.model.indexOf('/') + 1) || message.info.providerID !== this.model.split('/')[0])) throw new Error('Unexpected native assistant model')
        for (const part of message.parts ?? []) {
          if (part.sessionID !== sessionID) throw new Error('Cross-session native part rejected')
          if (part.messageID !== message.info.id || typeof part.id !== 'string' || partIDs.has(part.id)) throw new Error('Native message/part identity mismatch')
          partIDs.add(part.id)
          const type = part.type === 'tool' ? 'tool_use' : part.type === 'step-finish' ? 'step_finish' : part.type
          if (type === 'tool_use' && !['completed', 'error'].includes(part.state?.status)) incomplete = true
          // Native assistant creation time is Unix milliseconds for this response.
          events.push(JSON.stringify({ type, sessionID, timestamp: message.info.time?.created, part }))
        }
      }
      const stdout = events.join('\n') + '\n'
      this.save('stdout.jsonl', stdout)
      const { output: ignored, commands, usage, ...metrics } = sessionSummary(stdout)
      const output = typeof response.output === 'string' ? this.redact(response.output) : ''
      if (workspace !== this.isolated.workspace) fs.cpSync(workspace, this.isolated.workspace, { recursive: true })
      const metadata = { ...response.metadata, ...metrics, ...(this.telemetry ? { telemetry: this.telemetry.identity() } : {}), version: SUPPORTED_OPENCODE_VERSION, role: this.role, model: this.model, task: this.task, evidence: this.evidence, workspace, archivedWorkspace: this.isolated.workspace, isolationRoot: this.isolated.root, usage, elapsedSeconds: (performance.now() - this.started) / 1000, timedOut, timeoutMs: this.timeoutMs, exit: { code: response.error || incomplete ? 1 : 0, timedOut }, harnessBlocker: blocked, executionFailure: blocked ? 'harness' : response.error || incomplete || !output || timedOut ? 'model' : undefined }
      this.save('commands.json', commands)
      this.save('answer.txt', output)
      this.save('metrics.json', metadata)
      const safeResponse = JSON.parse(this.redact(JSON.stringify(response)))
      const error = blocked ? 'Harness blocker: viewer permission denial' : timedOut ? 'OpenCode session timed out' : incomplete || !output ? 'Incomplete OpenCode session' : safeResponse.error
      this.telemetryOutcome = { error, output }
      return { ...safeResponse, output, error, metadata, tokenUsage: { prompt: usage.input, completion: usage.output, cached: usage.cacheRead, total: usage.input + usage.output + usage.cacheRead } }
    } catch {
      const stdout = [...latestParts.values()].map(part => JSON.stringify({ type: part.type === 'tool' ? 'tool_use' : part.type === 'step-finish' ? 'step_finish' : part.type, sessionID, part })).join('\n') + '\n'
      const { output, commands, usage, ...metrics } = sessionSummary(stdout)
      const metadata = { ...metrics, usage, ...(this.telemetry ? { telemetry: this.telemetry.identity() } : {}), role: this.role, model: this.model, task: this.task, evidence: this.evidence, workspace, archivedWorkspace: this.isolated.workspace, isolationRoot: this.isolated.root, timedOut, timeoutMs: this.timeoutMs, elapsedSeconds: (performance.now() - this.started) / 1000, exit: { code: 1, timedOut }, harnessBlocker: blocked, executionFailure: timedOut && invoked && !monitorError && !blocked ? 'model' : 'harness' }
      for (const [name, value] of [['stdout.jsonl', stdout], ['commands.json', commands], ['answer.txt', output], ['metrics.json', metadata]] as const) if (!fs.existsSync(path.join(this.evidence, name))) this.save(name, value)
      if (!fs.existsSync(path.join(this.evidence, 'failure.json'))) this.save('failure.json', { ...metadata, error: 'Native session or evidence collection failed' })
      this.telemetryOutcome = { error: blocked ? 'Harness blocker: viewer permission denial' : timedOut ? 'OpenCode session timed out' : 'Native session or evidence collection failed; see ' + this.evidence }
      return { ...this.telemetryOutcome, metadata }
    } finally {
      clearTimeout(timer)
      if (escalation) clearTimeout(escalation)
      streamController.abort()
      await monitor?.catch(() => {})
      this.diagnostics()
      // Keep the attempt registered until afterEach sends any run-stop IPC.
      if (this.telemetry) await this.close(true)
    }
  }
  async close(retainForHook = false) {
    if (!retainForHook) attempts.delete(this.evidence)
    return this.closing ??= (async () => {
      try {
        if (this.telemetry && this.baseUrl && this.child?.exitCode === null && this.child.signalCode === null) {
          // Disposing the instance drains awaited plugin hooks and closes native
          // instance scopes before signalling the process. Keep the observer up.
          const response = await fetch(this.baseUrl + '/global/dispose', { method: 'POST', signal: AbortSignal.timeout(10000) })
          if (!response.ok) throw new Error('Native telemetry dispose failed')
          // Pinned native tracing has no public forceFlush API. Allow its 100ms
          // batch timer to run after disposal; native receipt is verified in the
          // integration check, not inferred from this delay.
          await new Promise(resolve => setTimeout(resolve, 250))
        }
      } catch (cause) {
        if (this.telemetry) this.telemetry.failure = this.redact(cause instanceof Error ? cause.message : 'Native telemetry cleanup failed')
        throw cause
      } finally {
        try { if (this.child) await stopOwned(this.child) }
        finally { await this.telemetry?.finish(this.telemetryOutcome) }
      }
      if (this.telemetry && (!this.telemetry.ready || this.telemetry.failure))
        throw new Error('Native telemetry hook failed; see ' + this.evidence)
    })()
  }
}

async function stopOwned(child: ReturnType<typeof spawn>) {
  if (!child.pid) return
  const signal = (value: NodeJS.Signals) => {
    try { if (process.platform === 'win32') child.kill(value); else process.kill(-child.pid!, value) }
    catch (error) { if (!(error instanceof Error && 'code' in error && error.code === 'ESRCH')) throw error }
  }
  const closed = child.exitCode !== null || child.signalCode !== null ? Promise.resolve() : once(child, 'close')
  signal('SIGTERM')
  let timer: ReturnType<typeof setTimeout>
  const grace = new Promise<void>(resolve => { timer = setTimeout(() => { signal('SIGKILL'); resolve() }, 30000) })
  await closed
  // Only a vanished process group proves that owned descendants have also exited.
  try {
    process.kill(process.platform === 'win32' ? child.pid : -child.pid, 0)
    await grace
  } catch (error) {
    if (!(error instanceof Error && 'code' in error && error.code === 'ESRCH')) throw error
    clearTimeout(timer!)
  }
}

export async function beforeEach(context: { test: TestCase }) {
  const file = process.env.OTEL_EVAL_ISOLATION_FILE
  if (!file || !path.isAbsolute(file) || fs.statSync(file).mode & 0o077) throw new Error('Private isolation settings required')
  const model = context.test.vars?.eval_model
  if (typeof model !== 'string') throw new Error('Explicit attempt model required')
  const attempt = await NativeAttempt.create(model, 'investigator', context.test.vars?.task_id, readJson(file))
  return { test: { ...context.test, provider: attempt, vars: { ...context.test.vars, binary: './bin/otel-desktop-viewer' }, metadata: { ...context.test.metadata, nativeAttempt: attempt.evidence } } }
}

export async function afterEach(context: { test: TestCase; result?: { failureReason?: number; response?: { error?: string; metadata?: Record<string, unknown> } } }) {
  const root = runtimeRoot()
  if (context.result?.failureReason === 2 || context.result?.response?.error) {
    recordExecutionError({
      model: context.test.vars?.eval_model,
      task: context.test.vars?.task_id, evidence: context.test.metadata?.nativeAttempt,
      modelFailure: context.result.response?.metadata?.executionFailure === 'model',
    })
  }
  const evidence = context.test.metadata?.nativeAttempt
  if (typeof evidence === 'string') {
    const attempt = attempts.get(evidence)
    if (attempt && fs.existsSync(path.join(root, 'harness-blocker.json')) && process.send) process.send({ type: 'otel-eval-harness-blocker', root, runnerPID: process.pid, sessionPID: attempt.child?.pid })
    try {
      await attempt?.close()
    } catch (error) {
      recordExecutionError({ model: context.test.vars?.eval_model, task: context.test.vars?.task_id, evidence, modelFailure: false })
      throw error
    }
  }
  return context
}

export async function afterAll() {
  for (const attempt of [...attempts.values()]) await attempt.close()
}
