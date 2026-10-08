import fs from 'node:fs'
import path from 'node:path'
import { spawn } from 'node:child_process'
import { readJson, record, runtimeRoot } from './runtime.ts'
import {
  SUPPORTED_OPENCODE_VERSION,
  createCleanEnvironment,
  credentialEnvironment,
} from './clean-environment.ts'
import { usageStep } from './pricing.ts'

export type Isolation = {
  permission?: unknown
  authFile?: string
  authEnvironmentFile?: string
  authEnvironment?: unknown
  providerIDs?: string[]
  executablePaths?: string[]
  opencodeBinary?: string
}

type ProcessResult = {
  stdout: string
  stderr: string
  code: number | null
  signal: NodeJS.Signals | null
  errorCode?: string
  pid?: number
}

type Event = {
  type: string
  sessionID?: string
  part: Record<string, unknown>
}

function parseEvents(stdout: string) {
  const events: Event[] = []
  for (const line of stdout.split('\n')) {
    try {
      const event = record(JSON.parse(line))
      if (typeof event.type !== 'string') continue
      events.push({
        type: event.type,
        sessionID:
          typeof event.sessionID === 'string' ? event.sessionID : undefined,
        part: record(event.part),
      })
    } catch {}
  }
  return events
}

const tokenCount = (value: unknown) =>
  typeof value === 'number' && Number.isFinite(value) ? value : 0

function sessionSummary(stdout: string) {
  const events = parseEvents(stdout)
  const steps = events.filter(event => event.type === 'step_finish')
  const usage = {
    input: 0,
    output: 0,
    reasoning: 0,
    cacheRead: 0,
    cacheWrite: 0,
  }
  const usageSteps = steps.map(step => usageStep(record(step.part.tokens)))
  for (const step of steps) {
    const tokens = record(step.part.tokens)
    const cache = tokens.cache === undefined ? {} : record(tokens.cache)
    usage.input += tokenCount(tokens.input)
    usage.output += tokenCount(tokens.output)
    usage.reasoning += tokenCount(tokens.reasoning)
    usage.cacheRead += tokenCount(cache.read)
    usage.cacheWrite += tokenCount(cache.write)
  }
  const lastStep = steps.at(-1)?.part
  const output = events
    .filter(
      event =>
        event.type === 'text' && event.part.messageID === lastStep?.messageID
    )
    .map(event => (typeof event.part.text === 'string' ? event.part.text : ''))
    .join('\n')
  const tools = events
    .filter(event => event.type === 'tool_use')
    .map(event => {
      const state = record(event.part.state)
      return {
        tool: event.part.tool,
        state,
        input: record(state.input),
        metadata: state.metadata === undefined ? {} : record(state.metadata),
      }
    })
  const commands = tools
    .filter(tool => tool.tool === 'bash')
    .map(tool => ({
      command: typeof tool.input.command === 'string' ? tool.input.command : '',
      status: tool.state.status,
      exit: tool.metadata.exit,
    }))
  return {
    output,
    usage,
    usageSteps,
    commands,
    reason: lastStep?.reason,
    sessionIDs: [
      ...new Set(
        events
          .map(event => event.sessionID)
          .filter(value => typeof value === 'string')
      ),
    ],
    toolCalls: tools.length,
    toolResponseBytes: tools.reduce(
      (bytes, tool) =>
        bytes +
        Buffer.byteLength(
          typeof tool.state.output === 'string' ? tool.state.output : ''
        ),
      0
    ),
    // Classify shell entries, not expanded invocations or loop iterations.
    utilityCommandEntries: commands.filter(command =>
      /otel-desktop-viewer["']?\s+(?:attributes|traces|trace|span|logs|metrics)\b/.test(
        command.command
      )
    ).length,
    queryCommandEntries: commands.filter(command =>
      /otel-desktop-viewer["']?\s+query\b/.test(command.command)
    ).length,
    failedToolCalls: tools.filter(
      tool =>
        tool.state.status === 'error' ||
        (typeof tool.metadata.exit === 'number' && tool.metadata.exit !== 0)
    ).length,
  }
}

function stringList(value: unknown) {
  if (!Array.isArray(value) || !value.every(item => typeof item === 'string'))
    throw new Error('Invalid string list')
  return value
}

export function isolationSettings(value: unknown): Isolation {
  const source = record(value)
  const settings: Isolation = {
    permission: source.permission,
    authEnvironment: source.authEnvironment,
  }
  for (const key of [
    'authFile',
    'authEnvironmentFile',
    'opencodeBinary',
  ] as const) {
    if (source[key] === undefined) continue
    if (typeof source[key] !== 'string')
      throw new Error('Invalid isolation file reference')
    settings[key] = source[key]
  }
  if (source.providerIDs !== undefined)
    settings.providerIDs = stringList(source.providerIDs)
  if (source.executablePaths !== undefined)
    settings.executablePaths = stringList(source.executablePaths)
  return settings
}

// Only launches a normal OpenCode CLI session. No custom agent, tool map,
// session reuse, rubric injection or command-selection instructions.
export default class OpenCodeProvider {
  model: string
  isolation: unknown
  constructor(options: { config: { model: string; isolation?: unknown } }) {
    this.model = options.config.model
    this.isolation = options.config.isolation
  }
  id() {
    return 'opencode-cli:' + this.model
  }
  async callApi(prompt: string, context: { vars: { task_id?: unknown } }) {
    const root = runtimeRoot()
    const workspaceRoot = path.join(root, 'workspaces')
    const evidenceRoot = path.join(root, 'runs')
    fs.mkdirSync(workspaceRoot, { recursive: true })
    fs.mkdirSync(evidenceRoot, { recursive: true })
    const tag =
      (this.model + '-' + context.vars.task_id).replace(
        /[^a-zA-Z0-9_.-]/g,
        '-'
      ) + '-'
    const evidence = fs.mkdtempSync(path.join(evidenceRoot, tag))
    fs.chmodSync(evidence, 0o700)
    const started = performance.now()
    let isolated: ReturnType<typeof createCleanEnvironment> | undefined
    let version: string | undefined
    let binary = ''
    let redact = (text: string) => text
    let secrets: string[] = []
    const save = (name: string, value: unknown) =>
      fs.writeFileSync(
        path.join(evidence, name),
        redact(
          typeof value === 'string' ? value : JSON.stringify(value, null, 2)
        ),
        { flag: 'wx', mode: 0o600 }
      )
    const execute = (args: string[], stdoutFile: string, stderrFile: string) =>
      new Promise<ProcessResult>(resolve => {
        if (!isolated) throw new Error('Isolation environment missing')
        const sink = (name: string) => {
          const descriptor = fs.openSync(path.join(evidence, name), 'wx', 0o600)
          let pending = ''
          const maximum = Math.max(1, ...secrets.map(value => value.length))
          return (chunk: string, final = false) => {
            pending += chunk
            const end = final
              ? pending.length
              : Math.max(0, pending.length - maximum + 1)
            let offset = 0,
              safe = ''
            // Retain a tail so a credential split across chunks is never written raw.
            while (offset < end) {
              const secret = secrets.find(value =>
                pending.startsWith(value, offset)
              )
              if (secret) {
                safe += '[REDACTED]'
                offset += secret.length
              } else {
                safe += pending[offset]
                offset++
              }
            }
            pending = pending.slice(offset)
            fs.writeSync(descriptor, safe)
            if (final) fs.closeSync(descriptor)
          }
        }
        const writeOut = sink(stdoutFile),
          writeErr = sink(stderrFile)
        const child = spawn(binary, args, {
          cwd: isolated.workspace,
          env: isolated.env,
          stdio: ['ignore', 'pipe', 'pipe'],
        })
        if (stdoutFile === 'stdout.jsonl' && child.pid)
          save('process.pid', String(child.pid))
        let stdout = '',
          stderr = '',
          errorCode: string | undefined
        child.stdout.setEncoding('utf8')
        child.stderr.setEncoding('utf8')
        child.stdout.on('data', data => {
          stdout += data
          writeOut(data)
        })
        child.stderr.on('data', data => {
          stderr += data
          writeErr(data)
        })
        child.once('error', error => {
          errorCode =
            'code' in error && typeof error.code === 'string'
              ? error.code
              : 'SPAWN_ERROR'
        })
        child.once('close', (code, signal) => {
          writeOut('', true)
          writeErr('', true)
          resolve({
            stdout: redact(stdout),
            stderr: redact(stderr),
            code,
            signal,
            errorCode,
            pid: child.pid,
          })
        })
      })
    try {
      if (!this.isolation)
        throw new Error(
          'Caller must supply isolation settings and approved permission rules'
        )
      const settings = isolationSettings(this.isolation)
      const {
        permission,
        authFile,
        authEnvironmentFile,
        providerIDs = [],
        executablePaths = [],
        opencodeBinary = 'opencode',
      } = settings
      if (settings.authEnvironment)
        throw new Error(
          'Caller must supply credential file references, not inline credentials'
        )
      for (const file of [authFile, authEnvironmentFile]) {
        if (!file) continue
        if (!path.isAbsolute(file) || fs.statSync(file).mode & 0o077) {
          throw new Error(
            'Caller must supply private absolute credential file references'
          )
        }
      }
      const authEnvironment = authEnvironmentFile
        ? credentialEnvironment(readJson(authEnvironmentFile))
        : {}
      binary = opencodeBinary
      secrets = Object.values(authEnvironment)
      if (authFile) {
        const auth = record(readJson(authFile))
        const collect = (value: unknown) => {
          if (typeof value === 'string') secrets.push(value)
          else if (value && typeof value === 'object')
            for (const item of Object.values(value)) collect(item)
        }
        for (const id of new Set([this.model.split('/')[0], ...providerIDs])) {
          const authRecord = auth[id]
          if (authRecord && typeof authRecord === 'object')
            for (const [key, value] of Object.entries(authRecord))
              if (key !== 'type') collect(value)
        }
      }
      secrets = secrets
        .filter(value => typeof value === 'string' && value.length)
        .sort((a, b) => b.length - a.length)
      redact = text => {
        for (const secret of secrets) {
          text = text.split(secret).join('[REDACTED]')
        }
        return text
      }
      isolated = createCleanEnvironment({
        parent: workspaceRoot,
        model: this.model,
        permission,
        authFile,
        authEnvironment,
        providerIDs,
        executablePaths: [root, ...executablePaths],
      })
      const candidates = path.isAbsolute(binary)
        ? [binary]
        : isolated.env.PATH.split(path.delimiter).map(directory =>
            path.join(directory, binary)
          )
      for (const candidate of candidates) {
        try {
          fs.accessSync(candidate, fs.constants.X_OK)
          if (fs.statSync(candidate).isFile()) {
            binary = fs.realpathSync(candidate)
            break
          }
        } catch {}
      }
      const checked = await execute(
        ['--version'],
        'version.stdout',
        'version.stderr'
      )
      version = checked.stdout.trim()
      if (checked.errorCode)
        throw new Error('OpenCode spawn failed: ' + checked.errorCode)
      if (checked.code !== 0 || version !== SUPPORTED_OPENCODE_VERSION)
        throw new Error(
          'Unsupported OpenCode version; required ' + SUPPORTED_OPENCODE_VERSION
        )
      let catalogue = await execute(
        ['models', this.model.split('/')[0]],
        'models.stdout',
        'models.stderr'
      )
      if (catalogue.errorCode || catalogue.code !== 0)
        throw new Error('Clean OpenCode model discovery failed')
      if (!catalogue.stdout.trim().split(/\r?\n/).includes(this.model)) {
        // A fresh CLI can load its bundled snapshot before its background refresh finishes.
        catalogue = await execute(
          ['models', this.model.split('/')[0], '--refresh'],
          'models-refreshed.stdout',
          'models-refreshed.stderr'
        )
        if (catalogue.errorCode || catalogue.code !== 0)
          throw new Error('Clean OpenCode model refresh failed')
      }
      if (!catalogue.stdout.trim().split(/\r?\n/).includes(this.model))
        throw new Error(
          'Requested model is absent from the clean catalogue: ' + this.model
        )
    } catch (error) {
      // Do not expose parser/input errors that can contain authentication material.
      const message = error instanceof Error ? error.message : ''
      const safe =
        /^(Caller must|Unsupported OpenCode|OpenCode spawn failed|Clean OpenCode|Requested model|Managed macOS|Remote-config|Only explicit|Executable search|An explicit)/.test(
          message
        )
          ? message
          : 'Isolation setup failed; check protected caller settings'
      const metadata = {
        model: this.model,
        task: context.vars.task_id,
        evidence,
        workspace: isolated?.workspace,
        isolationRoot: isolated?.root,
        version,
        elapsedSeconds: (performance.now() - started) / 1000,
      }
      save('failure.json', { error: safe, ...metadata })
      return { error: safe + '; see ' + evidence, metadata }
    }
    if (!isolated) throw new Error('Isolation environment missing')
    const workspace = isolated.workspace
    const supplied =
      prompt + '\n\nYour temporary working directory: ' + workspace + '\n'
    if (redact(supplied) !== supplied) {
      const metadata = {
        model: this.model,
        version,
        workspace,
        evidence,
        isolationRoot: isolated.root,
      }
      save('failure.json', {
        error: 'Prompt contains selected credential material',
        ...metadata,
      })
      return {
        error: 'Prompt contains selected credential material; see ' + evidence,
        metadata,
      }
    }
    try {
      const args = [
        'run',
        '--dir',
        workspace,
        '--agent',
        'build',
        '--model',
        this.model,
        '--format',
        'json',
        '--auto',
        supplied,
      ]
      save('prompt.txt', supplied)
      save('launch.json', {
        model: this.model,
        version,
        binary,
        workspace,
        isolationRoot: isolated.root,
        args,
        startedAt: new Date().toISOString(),
      })
      const result = await execute(args, 'stdout.jsonl', 'stderr.log')
      const exit = {
        code: result.code,
        signal: result.signal,
        errorCode: result.errorCode,
      }
      const { output, usage, commands, reason, ...metrics } = sessionSummary(
        result.stdout
      )
      const metadata = {
        model: this.model,
        version,
        binary,
        isolationRoot: isolated.root,
        task: context.vars.task_id,
        workspace,
        evidence,
        elapsedSeconds: (performance.now() - started) / 1000,
        exit,
        usage,
        ...metrics,
      }
      save('commands.json', commands)
      save('metrics.json', metadata)
      save('answer.txt', output)
      if (exit.code !== 0 || reason !== 'stop' || !output) {
        return {
          error: 'Incomplete OpenCode session; see ' + evidence,
          metadata,
        }
      }
      return {
        output,
        cached: false,
        metadata,
        tokenUsage: {
          prompt: usage.input,
          completion: usage.output,
          cached: usage.cacheRead,
          total: usage.input + usage.output + usage.cacheRead,
        },
      }
    } catch {
      const metadata = {
        model: this.model,
        version,
        binary,
        workspace,
        evidence,
        isolationRoot: isolated.root,
        task: context.vars.task_id,
        elapsedSeconds: (performance.now() - started) / 1000,
      }
      const error = 'OpenCode session processing failed'
      save('failure.json', { error, ...metadata })
      return { error: error + '; see ' + evidence, metadata }
    }
  }
}
