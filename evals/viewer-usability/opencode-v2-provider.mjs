import fs from 'node:fs'
import path from 'node:path'
import { homedir, tmpdir } from 'node:os'
import { spawn, execFileSync } from 'node:child_process'
import { once } from 'node:events'
import { pathToFileURL } from 'node:url'
import { setTimeout as delay } from 'node:timers/promises'
import { context as otelContext, propagation } from '@opentelemetry/api'
import { exportToolLogs, missingToolLogs } from './tool-logs.ts'
import { stopOwnedGroup } from './run-cancellation.ts'
import { aggregateUsage, toolEvidence } from './opencode-v2-evidence.mjs'
import { openV2EventStream } from './opencode-v2-stream.mjs'
import { nativeTelemetryReceiver, readNativeTelemetry, nativeTelemetryBoundaries } from './opencode-v2-telemetry.mjs'

export const V2_BINARY = '/Users/moya/Library/Application Support/ai.opencode.desktop/cli/2.0.29-arm64/opencode-cli'
const children = new Set()
export async function cancelOpenCodeV2() {
  await Promise.all([...children].map(child => stopOwnedGroup(child.pid, child)))
}
const json = file => JSON.parse(fs.readFileSync(file, 'utf8'))

export default class OpenCodeV2Provider {
  constructor({ config, label }) { this.config = config; this.label = label }
  id() { return 'opencode-v2:openai/' + this.config.model }

  async callApi(prompt, context = {}, options = {}) {
    const root = fs.realpathSync(this.config.root)
    for (let dir = root; ; dir = path.dirname(dir)) {
      if (fs.existsSync(path.join(dir, '.git'))) throw new Error('V2 evidence must be outside repositories')
      if (dir === path.dirname(dir)) break
    }
    fs.mkdirSync(path.join(root, 'runs'), { recursive: true, mode: 0o700 })
    const evidence = fs.mkdtempSync(path.join(root, 'runs/investigator-opencode-v2-'))
    fs.chmodSync(evidence, 0o700)
    let telemetry
    try {
      if (this.config.telemetryForward) telemetry = await nativeTelemetryReceiver(evidence, this.config.telemetryForward)
      return await this.callAttempt(prompt, context, options, root, evidence, telemetry?.endpoint ?? this.config.telemetryEndpoint, telemetry)
    } catch (error) {
      // Preflight/lifecycle failures must remain ordinary error rows, retaining
      // slot identity so subsequent scheduled repetitions can run normally.
      const carrier = {}; propagation.inject(otelContext.active(), carrier)
      const identityFile = path.join(evidence, 'identity.json')
      if (!fs.existsSync(identityFile)) fs.writeFileSync(identityFile, JSON.stringify({ task: context.vars?.task_id, provider: this.id(), role: 'investigator',
        promptfoo: { evaluationId: context.evaluationId, testIdx: context.testIdx, repeatIndex: context.repeatIndex, traceparent: carrier.traceparent ?? context.traceparent } }), { flag: 'wx', mode: 0o600 })
      const response = { error: error instanceof Error ? error.message : String(error), metadata: {
        runtime: 'opencode-v2', expectedRuntimeVersion: '2.0.29', role: 'investigator', model: 'openai/' + this.config.model,
        task: context.vars?.task_id, evidence, workspace: path.join(evidence, 'workspace'), repeatIndex: context.repeatIndex,
        exit: 'provider-exception', captureUnavailable: true,
      } }
      fs.writeFileSync(path.join(evidence, 'provider-exception.json'), JSON.stringify(response, null, 2), { flag: 'wx', mode: 0o600 })
      if (!fs.existsSync(path.join(evidence, 'response.json'))) fs.writeFileSync(path.join(evidence, 'response.json'), JSON.stringify(response, null, 2), { flag: 'wx', mode: 0o600 })
      return response
    } finally {
      if (telemetry) {
        try {
          await telemetry.close()
          const identity = json(path.join(evidence, 'identity.json'))
          fs.writeFileSync(path.join(evidence, 'native-telemetry-summary.json'), JSON.stringify(nativeTelemetryBoundaries(readNativeTelemetry(evidence, telemetry.receipts), identity.promptfoo.traceparent), null, 2), { flag: 'wx', mode: 0o600 })
        } catch (error) {
          fs.writeFileSync(path.join(evidence, 'native-telemetry-error.json'), JSON.stringify({ error: String(error), rawBytesRetained: true }), { flag: 'wx', mode: 0o600 })
        }
      }
    }
  }

  async callAttempt(prompt, context, options, root, evidence, telemetryEndpoint, telemetry) {
    const save = (name, value) => fs.writeFileSync(path.join(evidence, name), typeof value === 'string' ? value : JSON.stringify(value, null, 2) + '\n', { flag: 'wx', mode: 0o600 })
    const env = { PATH: '/opt/homebrew/bin:/usr/bin:/bin', SHELL: '/bin/sh', LANG: 'en_US.UTF-8',
      OPENCODE_CONFIG_PROJECT_DISABLE: '1', OPENCODE_FILEWATCHER_DISABLE: '1' }
    for (const [key, name] of Object.entries({ HOME: 'home', XDG_CONFIG_HOME: 'config', XDG_DATA_HOME: 'data', XDG_STATE_HOME: 'state', XDG_CACHE_HOME: 'cache', TMPDIR: 'tmp' })) {
      env[key] = path.join(evidence, name)
      fs.mkdirSync(env[key], { mode: 0o700 })
    }
    env.OPENCODE_TEST_HOME = env.HOME
    const workspace = path.join(evidence, 'workspace')
    fs.mkdirSync(path.join(workspace, 'bin'), { recursive: true, mode: 0o700 })
    fs.copyFileSync(this.config.binary, path.join(workspace, 'bin/otel-desktop-viewer'))
    fs.chmodSync(path.join(workspace, 'bin/otel-desktop-viewer'), 0o700)
    const binary = this.config.opencodeBinary ?? V2_BINARY
    const version = execFileSync(binary, ['--version'], { env, cwd: workspace, encoding: 'utf8', timeout: 10000 }).trim()
    if (version !== 'opencode v2.0.29') throw new Error('Only inspected OpenCode v2.0.29 is supported')
    const permissions = [
      { action: '*', resource: '*', effect: 'deny' },
      ...['read', 'edit', 'glob', 'grep'].map(action => ({ action, resource: '*', effect: 'allow' })),
      { action: 'external_directory', resource: '*', effect: 'deny' },
      ...['./bin/otel-desktop-viewer', path.join(workspace, 'bin/otel-desktop-viewer')].flatMap(executable =>
        ['--help', '--version', '-h', '-v', 'skills', 'logs', 'log', 'metrics', 'metric', 'traces', 'trace', 'span', 'attributes', 'services', 'query', 'export']
          .map(command => ({ action: 'shell', resource: executable + ' ' + command + ' *', effect: 'allow' }))),
    ]
    const plugin = path.join(evidence, 'policy')
    fs.mkdirSync(plugin, { mode: 0o700 })
    fs.copyFileSync(path.join(import.meta.dirname, 'opencode-v2-policy.mjs'), path.join(plugin, 'index.mjs'))
    save('policy/package.json', { name: 'viewer-eval-policy', private: true, type: 'module', exports: { '.': './index.mjs' } })
    env.OTEL_EVAL_V2_POLICY_LOG = path.join(evidence, 'policy.jsonl')
    fs.mkdirSync(path.join(env.XDG_CONFIG_HOME, 'opencode'), { recursive: true, mode: 0o700 })
    const config = {
      $schema: 'https://opencode.ai/config.json', update: 'disable', snapshots: false,
      model: 'openai/' + this.config.model, permissions, plugins: [pathToFileURL(plugin).href],
      warming: false, compaction: { auto: false },
      agents: { build: { permissions }, title: { disabled: true } },
      providers: { openai: { settings: { transport: 'http', ...(this.config.baseUrl ? { baseURL: this.config.baseUrl, apiKey: 'synthetic-only' } : {}) } } },
    }
    if (this.config.baseUrl) {
      if (new URL(this.config.baseUrl).hostname !== '127.0.0.1' || this.config.authFile) throw new Error('Synthetic service must be loopback without real auth')
      env.OPENAI_API_KEY = 'synthetic-only'
    }
    save('config/opencode/opencode.json', config)
    // OS boundary supplements native tool permissions. Runtime data is private
    // to this attempt; the model's narrow shell rule exposes only the viewer.
    const repo = fs.realpathSync(path.join(import.meta.dirname, '../..'))
    const boundaries = [...new Set([fs.realpathSync(homedir()), fs.realpathSync(tmpdir()), repo, root])]
    const policy = '(version 1)\n(allow default)\n' + boundaries.map(boundary =>
      `(deny file-read-data (require-all (subpath ${JSON.stringify(boundary)}) (require-not (subpath ${JSON.stringify(evidence)})) (require-not (literal ${JSON.stringify(binary)}))))\n` +
      `(deny file-write* (require-all (subpath ${JSON.stringify(boundary)}) (require-not (subpath ${JSON.stringify(evidence)}))))\n`
    ).join('')
    save('sandbox.sb', policy)
    const invocation = otelContext.active()
    const carrier = {}; propagation.inject(otelContext.active(), carrier)
    const traceparent = carrier.traceparent ?? context.traceparent
    if (traceparent && !/^00-[a-f0-9]{32}-[a-f0-9]{16}-0[01]$/.test(traceparent)) throw new Error('Invalid evaluation traceparent')
    if (telemetryEndpoint) {
      const endpoint = new URL(telemetryEndpoint)
      if (endpoint.protocol !== 'http:' || endpoint.hostname !== '127.0.0.1' || endpoint.pathname !== '/') throw new Error('Native telemetry requires an owned loopback receiver')
      env.OTEL_EXPORTER_OTLP_ENDPOINT = endpoint.origin
      env.OPENCODE_CLIENT = 'viewer-eval-v2'
      env.OTEL_RESOURCE_ATTRIBUTES = 'eval.runtime=opencode-v2,eval.evidence.id=' + path.basename(evidence)
      save('native-telemetry.json', { endpoint: endpoint.origin, environment: { OTEL_EXPORTER_OTLP_ENDPOINT: endpoint.origin,
        OPENCODE_CLIENT: env.OPENCODE_CLIENT, OTEL_RESOURCE_ATTRIBUTES: env.OTEL_RESOURCE_ATTRIBUTES },
        traceparent, source: 'OpenCode 2.0.29 built-in observability layer; no plugin-generated spans' })
    }
    save('identity.json', { task: context.vars?.task_id, provider: this.id(), role: 'investigator', promptfoo: {
      evaluationId: context.evaluationId, testIdx: context.testIdx, repeatIndex: context.repeatIndex,
      traceparent,
    } })
    save('prompt.txt', prompt)
    const started = performance.now()
    let child, closed, url, password, sessionID, stream, consume, history = [], receivedError, timedOut = false
    const secrets = []
    const redact = text => { for (const value of secrets) if (value) text = text.split(value).join('[REDACTED]'); return text }
    const streamController = new AbortController()
    const deadline = new AbortController()
    const abort = () => deadline.abort()
    options.abortSignal?.addEventListener('abort', abort, { once: true })
    const timer = setTimeout(() => { timedOut = true; deadline.abort() }, this.config.timeoutMs ?? 600000)
    const api = async (route, method = 'GET', body, name, signal = deadline.signal) => {
      const response = await fetch(url + route, { method, headers: { Authorization: 'Basic ' + Buffer.from('opencode:' + password).toString('base64'), 'Content-Type': 'application/json', ...(telemetryEndpoint && traceparent ? { traceparent } : {}) },
        ...(body === undefined ? {} : { body: JSON.stringify(body) }), signal })
      const raw = await response.text()
      if (name) save(name, redact(raw))
      if (!response.ok) throw new Error(`OpenCode HTTP ${response.status} ${route}: ${redact(raw)}`)
      return raw ? JSON.parse(raw) : undefined
    }
    try {
      if (options.abortSignal?.aborted) throw new Error('Evaluation cancelled before startup')
      child = spawn('/usr/bin/sandbox-exec', ['-f', path.join(evidence, 'sandbox.sb'), binary, 'serve', '--hostname', '127.0.0.1', '--port', '0'], {
        env, cwd: workspace, detached: true, stdio: ['ignore', 'pipe', 'pipe'],
      })
      children.add(child)
      closed = once(child, 'close')
      let stdout = ''
      child.stderr.on('data', chunk => fs.appendFileSync(path.join(evidence, 'server.stderr.log'), redact(String(chunk)), { mode: 0o600 }))
      await new Promise((resolve, reject) => {
        const timeout = setTimeout(() => reject(new Error('Private V2 server startup timed out')), 30000)
        child.stdout.on('data', chunk => {
          stdout += chunk
          url = stdout.match(/https?:\/\/127\.0\.0\.1:\d+/)?.[0]
          password = stdout.match(/server password\s+(\S+)/)?.[1]
          if (url && password) { secrets.push(password); clearTimeout(timeout); resolve() }
        })
        child.once('error', error => { clearTimeout(timeout); reject(error) })
        closed.then(() => { clearTimeout(timeout); reject(new Error('Private V2 server exited during startup')) }, reject)
      })
      save('server.stdout.log', redact(stdout))
      const info = await api('/api/info', 'GET', undefined, 'server-info.json')
      if (info.version !== '2.0.29') throw new Error('Wrong private server version')
      const entries = await api('/api/config', 'GET', undefined, 'effective-config.json')
      if (entries.some(entry => typeof entry.path !== 'string' || !entry.path.startsWith(evidence + '/'))) throw new Error('Ambient configuration leaked into V2')
      await api('/api/plugin')
      for (let attempt = 0; attempt < 100 && !fs.existsSync(env.OTEL_EVAL_V2_POLICY_LOG); attempt++) await delay(100)
      const plugins = await api('/api/plugin', 'GET', undefined, 'plugins.json')
      if (!plugins.data.some(plugin => plugin.id === 'viewer-eval-policy' && plugin.state.status === 'active')) throw new Error('V2 policy plugin not active')
      if (plugins.data.some(plugin => plugin.source.type !== 'builtin' && plugin.id !== 'viewer-eval-policy')) throw new Error('Ambient plugin leaked into V2')
      const skills = await api('/api/skill', 'GET', undefined, 'skills.json')
      if (skills.data?.length) throw new Error('Ambient skills leaked into V2')
      const mcp = await api('/api/mcp', 'GET', undefined, 'mcp.json')
      if (mcp.data?.length) throw new Error('Ambient MCP server leaked into V2')
      if (this.config.authFile) {
        const file = this.config.authFile
        if (!path.isAbsolute(file) || fs.statSync(file).mode & 0o077) throw new Error('V2 auth source must be private and absolute')
        const authEntry = this.config.authEntry ?? 'openai'
        if (!['openai', 'openai-codex'].includes(authEntry)) throw new Error('Unsupported explicit OpenAI OAuth source entry')
        const auth = json(file)[authEntry]
        if (auth?.type !== 'oauth') throw new Error('Selected OpenAI OAuth credential missing')
        save('authentication.json', { sourceEntry: authEntry, type: auth.type, expires: auth.expires, expired: auth.expires <= Date.now(), methodID: 'chatgpt-browser' })
        secrets.push(auth.access, auth.refresh)
        // Import the selected existing login into this server's private database.
        // Never persist the credential response or write to the original auth file.
        await api('/api/credential', 'POST', { integrationID: 'openai', label: 'isolated-eval', value: {
          type: 'oauth', methodID: 'chatgpt-browser', access: auth.access, refresh: auth.refresh, expires: auth.expires,
          metadata: { accountId: auth.accountId },
        } })
      }
      const models = await api('/api/model', 'GET', undefined, 'models.json')
      const catalogue = models.data ?? models
      if (!Array.isArray(catalogue) || !catalogue.some(model => model.providerID === 'openai' && model.id === this.config.model)) throw new Error('Requested model absent in isolated V2 catalogue')
      const created = await api('/api/session', 'POST', { title: 'Viewer checkout-failure evaluation', location: { directory: workspace }, agent: 'build', model: { providerID: 'openai', id: this.config.model }, permissions }, 'session-created.json')
      sessionID = created.data.id
      stream = await openV2EventStream(url, { headers: { Authorization: 'Basic ' + Buffer.from('opencode:' + password).toString('base64') },
        deadlineSignal: deadline.signal, cancellationSignal: options.abortSignal, stopSignal: streamController.signal })
      if (!stream.ok || !stream.body) throw new Error('V2 event stream unavailable')
      consume = (async () => {
        for await (const chunk of stream.body.pipeThrough(new TextDecoderStream())) fs.appendFileSync(path.join(evidence, 'events.sse'), redact(chunk), { mode: 0o600 })
      })().catch(error => { if (!streamController.signal.aborted) { receivedError = String(error); deadline.abort() } })
      await api('/api/session/' + sessionID + '/prompt', 'POST', { text: prompt }, 'admitted-prompt.json')
      await api('/api/experimental/session/' + sessionID + '/wait', 'POST')
      const result = await api('/api/session/' + sessionID + '/context', 'GET', undefined, 'context.json')
      history = result.data
      await api('/api/experimental/session/' + sessionID + '/export', 'GET', undefined, 'session-export.json')
      if (receivedError) throw new Error('Event capture failed: ' + receivedError)
    } catch (error) {
      receivedError = redact(error instanceof Error ? error.message : String(error))
      if (sessionID) {
        try {
          await api('/api/session/' + sessionID + '/interrupt', 'POST', undefined, 'interrupt.json', AbortSignal.timeout(10000))
          if (!fs.existsSync(path.join(evidence, 'context.json'))) history = (await api('/api/session/' + sessionID + '/context', 'GET', undefined, 'context.json', AbortSignal.timeout(10000))).data
        } catch (captureError) { save('capture-error.json', { error: redact(String(captureError)) }) }
      }
    } finally {
      clearTimeout(timer)
      options.abortSignal?.removeEventListener('abort', abort)
      streamController.abort()
      await consume
      if (child) {
        await stopOwnedGroup(child.pid, child)
        children.delete(child)
        save('owned-shutdown.json', { pid: child.pid, exit: await closed })
      }
    }
    const assistants = history.filter(message => message.type === 'assistant')
    const final = assistants.at(-1)
    const idle = history.findLast(message => message.type === 'idle')
    const output = final?.content.filter(part => part.type === 'text').map(part => part.text).join('\n') ?? ''
    const tools = assistants.flatMap(message => message.content.filter(part => part.type === 'tool').map(part => ({ messageID: message.id, sessionID, ...part })))
    save('tools.json', tools)
    const errors = history.flatMap(message => message.error ? [{ messageID: message.id, ...message.error }] : [])
    const harnessBlocker = tools.some(tool => tool.name === 'shell' && tool.state.error?.type === 'permission.rejected')
    const modelMismatch = assistants.some(message => message.model.providerID !== 'openai' || message.model.id !== this.config.model)
    const { usage, usageAvailability, tokenUsage } = aggregateUsage(assistants)
    const evidenceErrors = []
    const observedTimestamp = Date.now()
    const toolLogs = tools.map(tool => {
      const event = toolEvidence(tool)
      evidenceErrors.push(...event.errors)
      return { eventName: 'tool.execution', attributes: event.attributes, timestamp: event.timestamp, observedTimestamp }
    })
    save('tool-logs.json', toolLogs)
    await telemetry?.flush()
    const nativeSpans = telemetry ? readNativeTelemetry(evidence, telemetry.receipts).spans : []
    const enrichment = traceparent ? missingToolLogs(toolLogs, traceparent.split('-')[1], nativeSpans) : toolLogs
    await exportToolLogs(enrichment, invocation, 'OpenCode', path.basename(evidence))
    const metadata = { runtime: 'opencode-v2', runtimeVersion: '2.0.29', model: 'openai/' + this.config.model,
      role: 'investigator', task: context.vars?.task_id, evidence, workspace, sessionID, repeatIndex: context.repeatIndex, usage, usageAvailability, evidenceErrors,
      elapsedSeconds: (performance.now() - started) / 1000, toolCalls: tools.length, failedToolCalls: tools.filter(tool => tool.state.status === 'error').length,
      exit: idle?.outcome ?? 'incomplete', errors, harnessBlocker,
    }
    const failed = receivedError || timedOut || errors.length || evidenceErrors.length || harnessBlocker || modelMismatch || final?.finish !== 'stop' || idle?.outcome !== 'succeeded' || !output || tools.some(tool => !['completed', 'error'].includes(tool.state.status))
    const response = { ...(output ? { output } : {}), metadata, ...(tokenUsage ? { tokenUsage } : {}),
      ...(assistants.length && assistants.every(message => typeof message.cost === 'number') ? { cost: assistants.reduce((total, message) => total + message.cost, 0) } : {}),
      ...(failed ? { error: receivedError || JSON.stringify(errors.length ? errors : evidenceErrors.length ? { reason: 'Malformed V2 tool evidence', evidenceErrors } : { reason: harnessBlocker ? 'Viewer shell permission blocker' : modelMismatch ? 'Unexpected V2 model identity' : 'Incomplete V2 session', finish: final?.finish, idle: idle?.outcome, timedOut }) } : {}),
    }
    if (output) save('answer.txt', output)
    save('response.json', response)
    return response
  }
}
