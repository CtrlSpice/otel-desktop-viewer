import fs from 'node:fs'
import path from 'node:path'
import { homedir, tmpdir } from 'node:os'
import { spawn, type ChildProcess } from 'node:child_process'
import { StringDecoder } from 'node:string_decoder'
import { context as otelContext, propagation } from '@opentelemetry/api'
import { record, saveJson, SUITE } from './runtime.ts'
import { stopOwnedGroup } from './run-cancellation.ts'

export type PiConfig = {
  root: string
  provider: string
  model: string
  authFile?: string
  binary?: string
  role: 'investigator' | 'grader'
  timeoutMs?: number
  /** Scripted loopback endpoint for integration tests. */
  baseUrl?: string
}
export type PiContext = {
  vars?: Record<string, unknown>
  evaluationId?: string
  testIdx?: number
  repeatIndex?: number
  traceparent?: string
}
const children = new Set<ChildProcess>()
export async function cancelPiAttempts() {
  await Promise.all([...children].map(child => stopOwnedGroup(child.pid!, child)))
}

export default class PiProvider {
  config: PiConfig
  label?: string
  constructor({ config, label }: { config: PiConfig; label?: string }) {
    this.config = config
    this.label = label
  }
  id() { return `pi:${this.config.provider}/${this.config.model}` }

  async callApi(prompt: string, context: PiContext = {}) {
    if (process.platform !== 'darwin') throw new Error('Pi evaluation currently requires the tested macOS sandbox')
    const { provider, model, role } = this.config
    const root = fs.realpathSync(this.config.root)
    const runs = path.join(root, 'runs')
    fs.mkdirSync(runs, { recursive: true, mode: 0o700 })
    const evidence = fs.mkdtempSync(path.join(runs, role + '-'))
    fs.chmodSync(evidence, 0o700)
    for (const name of ['home', 'profile', 'workspace', 'sessions', 'tmp'])
      fs.mkdirSync(path.join(evidence, name), { mode: 0o700 })
    const workspace = path.join(evidence, 'workspace')
    const write = (name: string, text: string) => fs.writeFileSync(path.join(evidence, name), text, { mode: 0o600, flag: 'wx' })
    const carrier: Record<string, string> = {}
    propagation.inject(otelContext.active(), carrier)
    const promptfoo = { evaluationId: context.evaluationId, testIdx: context.testIdx, repeatIndex: context.repeatIndex,
      traceparent: carrier.traceparent ?? context.traceparent }
    const identity = { task: context.vars?.task_id, provider: this.id(), role }
    saveJson(path.join(evidence, 'identity.json'), { ...identity, promptfoo })
    write('prompt.txt', prompt)
    saveJson(path.join(evidence, 'profile/settings.json'), {
      retry: { enabled: false }, compaction: { enabled: false }, cacheWarming: 'off', shellPath: '/bin/sh',
    })
    if (role === 'investigator' && this.config.binary) {
      fs.mkdirSync(path.join(workspace, 'bin'), { mode: 0o700 })
      fs.copyFileSync(this.config.binary, path.join(workspace, 'bin/otel-desktop-viewer'))
      fs.chmodSync(path.join(workspace, 'bin/otel-desktop-viewer'), 0o700)
    }
    if (this.config.baseUrl) {
      if (!['127.0.0.1', 'localhost', '[::1]'].includes(new URL(this.config.baseUrl).hostname) || this.config.authFile)
        throw new Error('Scripted endpoint must be loopback and cannot receive saved credentials')
      saveJson(path.join(evidence, 'profile/models.json'), {
        providers: { [provider]: { baseUrl: this.config.baseUrl, api: 'openai-completions', apiKey: 'local-fixture', models: [{ id: model, reasoning: false, input: ['text'], contextWindow: 128000, maxTokens: 4096, cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 } }] } },
      })
    } else {
      const file = this.config.authFile
      if (!file || !path.isAbsolute(file) || fs.statSync(file).mode & 0o077)
        throw new Error('Pi requires a private absolute auth file')
      let auth: Record<string, unknown>
      try { auth = record(JSON.parse(fs.readFileSync(file, 'utf8'))) }
      catch { throw new Error('Cannot parse Pi auth file') }
      if (!auth[provider]) throw new Error('Missing selected Pi provider credential')
      saveJson(path.join(evidence, 'profile/auth.json'), { [provider]: auth[provider] })
    }
    const modules = fs.realpathSync(path.join(SUITE, 'node_modules'))
    const repo = fs.realpathSync(path.join(SUITE, '../..'))
    // Keep the development tree, fixture, rubrics and other Rep evidence out of
    // the agent filesystem. Node still needs metadata reads for module resolution.
    const boundaries = [...new Set([fs.realpathSync(homedir()), fs.realpathSync(tmpdir()), repo, root])]
    const policy = `(version 1)\n(allow default)\n` + boundaries.map(boundary =>
      `(deny file-read-data (require-all (subpath ${JSON.stringify(boundary)}) (require-not (subpath ${JSON.stringify(evidence)})) (require-not (subpath ${JSON.stringify(modules)}))))\n` +
      `(deny file-write* (require-all (subpath ${JSON.stringify(boundary)}) (require-not (subpath ${JSON.stringify(evidence)}))))\n`
    ).join('')
    write('sandbox.sb', policy)
    const env = {
      HOME: path.join(evidence, 'home'), XDG_CONFIG_HOME: path.join(evidence, 'home/config'),
      XDG_DATA_HOME: path.join(evidence, 'home/data'), XDG_CACHE_HOME: path.join(evidence, 'home/cache'),
      TMPDIR: path.join(evidence, 'tmp'), PATH: '/opt/homebrew/bin:/usr/bin:/bin', SHELL: '/bin/sh', LANG: 'en_US.UTF-8',
      PI_CODING_AGENT_DIR: path.join(evidence, 'profile'), PI_OFFLINE: '1', PI_SKIP_VERSION_CHECK: '1', PI_TELEMETRY: '0',
    }
    const args = ['-f', path.join(evidence, 'sandbox.sb'), process.execPath,
      path.join(modules, '@earendil-works/pi-coding-agent/dist/bundle/cli.js'),
      '--mode', 'json', '--provider', provider, '--model', model, '--thinking', 'low',
      '--session-dir', path.join(evidence, 'sessions'), '--no-context-files', '--no-extensions',
      '--no-mcp', '--no-skills', '--no-prompt-templates', '--no-themes', '--no-approve',
      ...(role === 'grader' ? ['--no-tools'] : ['--tools', 'read,bash,edit,write']), '--', prompt]
    const events: Record<string, unknown>[] = []
    const decoder = new StringDecoder('utf8')
    let pending = '', malformed = false
    const consume = (text: string) => {
      pending += text
      let end: number
      while ((end = pending.indexOf('\n')) !== -1) {
        const line = pending.slice(0, end).replace(/\r$/, '')
        pending = pending.slice(end + 1)
        if (line) try { events.push(record(JSON.parse(line))) } catch { malformed = true }
      }
    }
    const started = performance.now()
    const child = spawn('/usr/bin/sandbox-exec', args, { cwd: workspace, env, detached: true, stdio: ['ignore', 'pipe', 'pipe'] })
    children.add(child)
    child.stdout.on('data', (chunk: Buffer) => {
      fs.appendFileSync(path.join(evidence, 'events.jsonl'), chunk, { mode: 0o600 })
      consume(decoder.write(chunk))
    })
    child.stderr.on('data', (chunk: Buffer) => fs.appendFileSync(path.join(evidence, 'stderr.log'), chunk, { mode: 0o600 }))
    let timedOut = false
    const timer = setTimeout(() => {
      timedOut = true
      if (child.pid) try { process.kill(-child.pid, 'SIGKILL') } catch {}
    }, this.config.timeoutMs ?? 600_000)
    let exit: number | null
    try { exit = await new Promise<number | null>((resolve, reject) => { child.once('error', reject); child.once('close', resolve) }) }
    finally { clearTimeout(timer); children.delete(child) }
    consume(decoder.end())
    const messages = events.filter(event => event.type === 'message_end').map(event => record(event.message))
    const message = messages.findLast(message => message.role === 'assistant')
    const content = Array.isArray(message?.content) ? message.content.map(record) : []
    const output = content.filter(block => block.type === 'text').map(block => block.text).join('\n')
    const settled = events.findLast(event => event.type === 'agent_settled')
    const metadata = {
      ...identity, runtime: 'pi', runtimeVersion: '1.3.0', model: `${provider}/${model}`,
      evidence, workspace, sessionID: events.find(event => event.type === 'session')?.id,
      elapsedSeconds: (performance.now() - started) / 1000, exit,
      toolCalls: events.filter(event => event.type === 'tool_execution_start').length,
      failedToolCalls: events.filter(event => event.type === 'tool_execution_end' && event.isError).length,
    }
    const response = exit !== 0 || timedOut || malformed || pending.trim() || !settled || settled.aborted || message?.stopReason !== 'stop' || !output
      ? { error: `Pi ${role} execution failed${timedOut ? ' (timeout)' : ''}; see ${evidence}`, metadata }
      : { output, metadata }
    if (response.output) write('answer.txt', response.output)
    saveJson(path.join(evidence, 'response.json'), response)
    return response
  }
}
