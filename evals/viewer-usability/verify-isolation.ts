import fs from 'node:fs'
import path from 'node:path'
import crypto from 'node:crypto'
import http from 'node:http'
import { spawnSync, spawn } from 'node:child_process'
import { DatabaseSync } from 'node:sqlite'
import { once } from 'node:events'
import Provider from './provider.ts'
import {
  cleanEnvironment,
  createCleanEnvironment,
  SUPPORTED_OPENCODE_VERSION,
} from './clean-environment.ts'
import { readJson, record, SUITE, TEMPORARY } from './runtime.ts'
import { readIsolationCheckSettings } from './isolation-check-settings.ts'

const {
  originalSuite,
  authFile,
  personalConfig,
  opencodeBinary: binary,
  viewerBinary: viewer,
  forbiddenMarkers,
} = readIsolationCheckSettings()

type Fingerprint =
  | { link: string }
  | { sha256: string; mode: number; size: number; mtimeMs: number }

function fingerprint(file: string, result: Record<string, Fingerprint> = {}) {
  const stat = fs.lstatSync(file)
  if (stat.isSymbolicLink()) result[file] = { link: fs.readlinkSync(file) }
  else if (stat.isDirectory()) {
    if (['node_modules', '.git'].includes(path.basename(file))) return result
    for (const entry of fs.readdirSync(file).sort())
      fingerprint(path.join(file, entry), result)
  } else
    result[file] = {
      sha256: crypto
        .createHash('sha256')
        .update(fs.readFileSync(file))
        .digest('hex'),
      mode: stat.mode & 0o777,
      size: stat.size,
      mtimeMs: stat.mtimeMs,
    }
  return result
}

const originals = () => ({
  ...fingerprint(personalConfig),
  ...fingerprint(authFile),
  ...fingerprint(originalSuite),
})

function credentialStrings(value: unknown, includeAccount = false): string[] {
  if (!value || typeof value !== 'object') return []
  return Object.entries(value).flatMap(([key, entry]) => {
    const pattern = includeAccount
      ? /key|token|access|refresh|secret|account/i
      : /key|token|access|refresh|secret/i
    return typeof entry === 'string' && pattern.test(key)
      ? [entry]
      : credentialStrings(entry, includeAccount)
  })
}

function protectedWriter(directory: string, secrets: string[] = []) {
  return (name: string, value: unknown) => {
    let text =
      typeof value === 'string' ? value : JSON.stringify(value, null, 2)
    for (const secret of secrets
      .filter(Boolean)
      .sort((a, b) => b.length - a.length))
      text = text.split(secret).join('[REDACTED]')
    fs.writeFileSync(path.join(directory, name), text, {
      flag: 'wx',
      mode: 0o600,
    })
  }
}

async function canary(
  parent: string,
  save: ReturnType<typeof protectedWriter>
) {
  const marker = 'EVAL_INTEGRATION_CONTEXT_CANARY_01983'
  const clean = createCleanEnvironment({
    parent,
    model: 'openai/gpt-4o',
    permission: { bash: 'ask' },
  })
  fs.writeFileSync(path.join(clean.workspace, 'AGENTS.md'), marker, {
    flag: 'wx',
    mode: 0o600,
  })
  let captured: unknown
  const server = http.createServer((req, res) => {
    let body = ''
    req.on('data', chunk => {
      body += chunk
    })
    req.on('end', () => {
      try {
        captured = JSON.parse(body)
      } catch {
        captured = undefined
      }
      res.writeHead(400, { 'content-type': 'application/json' })
      res.end(
        JSON.stringify({
          error: {
            message: 'Synthetic loader check; no model inference',
            type: 'invalid_request_error',
          },
        })
      )
    })
  })
  server.listen(0, '127.0.0.1')
  await once(server, 'listening')
  try {
    const address = server.address()
    if (!address || typeof address === 'string')
      throw new Error('No canary address')
    fs.writeFileSync(
      path.join(clean.env.OPENCODE_CONFIG_DIR, 'opencode.json'),
      JSON.stringify({
        ...clean.config,
        provider: {
          openai: {
            options: {
              apiKey: 'synthetic-canary-only',
              baseURL: `http://127.0.0.1:${address.port}/v1`,
            },
          },
        },
      }),
      { mode: 0o600 }
    )
    async function request(env: NodeJS.ProcessEnv, name: string) {
      captured = undefined
      const child = spawn(
        binary,
        [
          'run',
          '--dir',
          clean.workspace,
          '--agent',
          'build',
          '--model',
          'openai/gpt-4o',
          '--format',
          'json',
          'Reply OK.',
        ],
        { cwd: clean.workspace, env, stdio: ['ignore', 'pipe', 'pipe'] }
      )
      let stdout = '',
        stderr = ''
      child.stdout.on('data', data => {
        stdout += data
      })
      child.stderr.on('data', data => {
        stderr += data
      })
      const timer = setTimeout(() => child.kill('SIGTERM'), 60000)
      try {
        await once(child, 'close')
      } finally {
        clearTimeout(timer)
      }
      save(name + '-stdout.jsonl', stdout)
      save(name + '-stderr.log', stderr)
      if (!captured) throw new Error('Synthetic request was not captured')
      save(name + '-request.json', captured)
      return JSON.stringify(captured)
    }
    const control = await request(
      { ...clean.env, OPENCODE_DISABLE_PROJECT_CONFIG: '0' },
      'canary-control'
    )
    const isolated = await request(clean.env, 'canary-isolated')
    const forbidden = [
      ...forbiddenMarkers,
      'tasks.json',
      'grade.ts',
      'grade.cjs',
    ]
    const matches = forbidden.filter(value => isolated.includes(value))
    if (
      !control.includes(marker) ||
      isolated.includes(marker) ||
      matches.length
    )
      throw new Error('Context canary failed')
    if (
      fs.readdirSync(path.join(clean.env.OPENCODE_CONFIG_DIR, 'node_modules'))
        .length
    )
      throw new Error('Canary installed packages')
    const tools = record(JSON.parse(isolated)).tools
    if (!Array.isArray(tools)) throw new Error('Invalid canary tools')
    return {
      controlLoaded: true,
      isolatedLoaded: false,
      forbiddenMatches: matches,
      toolNames: tools.map(value => {
        const tool = record(value)
        return tool.name || record(tool.function).name
      }),
      method: 'Actual outbound synthetic HTTP requests; no external inference',
    }
  } finally {
    await new Promise<void>(resolve => server.close(() => resolve()))
  }
}

async function main() {
  const evidence = fs.mkdtempSync(
    path.join(TEMPORARY, 'eval-isolation-integration-verification-')
  )
  fs.chmodSync(evidence, 0o700)
  const before = originals()
  const save = protectedWriter(
    evidence,
    credentialStrings(readJson(authFile), true)
  )
  save('original-fingerprints-before.json', before)
  const summary: Record<string, unknown> = {
    evidence,
    binary,
    realBinary: fs.realpathSync(binary),
    requestedModel: 'openai/gpt-6.1-sol',
    externalInferenceAttempted: false,
    ambientProviderKeys: Object.keys(process.env).filter(key =>
      /^(OPENAI_|OPENCODE_MODELS_)/.test(key)
    ),
  }
  try {
    const model = 'openai/gpt-6.1-sol'
    const clean = createCleanEnvironment({
      parent: evidence,
      model,
      authFile,
      permission: {
        bash: { '*': 'allow', '*otel-desktop-viewer*': 'deny' },
        task: 'deny',
      },
    })
    summary.root = clean.root
    summary.workspace = clean.workspace
    const cli = (executable: string, args: string[], name: string) => {
      const result = spawnSync(executable, args, {
        cwd: clean.workspace,
        env: clean.env,
        encoding: 'utf8',
        timeout: 120000,
        maxBuffer: 32 * 1024 * 1024,
      })
      save(name + '.stdout', result.stdout || '')
      save(name + '.stderr', result.stderr || '')
      if (result.error || result.status !== 0)
        throw new Error('CLI check failed: ' + name)
      return result.stdout
    }
    summary.version = cli(binary, ['--version'], 'version').trim()
    if (summary.version !== SUPPORTED_OPENCODE_VERSION)
      throw new Error('Unsupported CLI version')
    const config = record(
      JSON.parse(cli(binary, ['debug', 'config'], 'config'))
    )
    for (const key of [
      'instructions',
      'agent',
      'mode',
      'plugin',
      'command',
      'mcp',
      'references',
      'skills',
    ]) {
      const value = config[key]
      if (value && typeof value === 'object' && Object.keys(value).length)
        throw new Error('Unexpected custom context: ' + key)
    }
    const paths = Object.fromEntries(
      cli(binary, ['debug', 'paths'], 'paths')
        .trim()
        .split('\n')
        .map(line =>
          line
            .trim()
            .split(/\s+(.+)/)
            .slice(0, 2)
        )
    )
    summary.paths = paths
    if (
      !Object.values(paths).every(
        value =>
          typeof value === 'string' && value.startsWith(clean.root + path.sep)
      )
    )
      throw new Error('Path escaped isolation root')
    const models = cli(binary, ['models', 'openai'], 'models')
      .trim()
      .split(/\r?\n/)
    summary.models = models
    summary.requestedModelPresent = models.includes(model)
    const refreshed = cli(
      binary,
      ['models', 'openai', '--refresh'],
      'models-refreshed'
    )
    summary.requestedModelPresentAfterRefresh = refreshed
      .trim()
      .split(/\r?\n/)
      .includes(model)
    const catalogueFile = path.join(
      clean.env.XDG_CACHE_HOME,
      'opencode/models.json'
    )
    summary.refreshedCatalogueFilePresent = fs.existsSync(catalogueFile)
    if (fs.existsSync(catalogueFile)) {
      const catalogue = record(readJson(catalogueFile))
      summary.sol61InRefreshedSource = Boolean(
        record(record(catalogue.openai).models)['gpt-6.1-sol']
      )
    }
    const agent = record(
      JSON.parse(cli(binary, ['debug', 'agent', 'build'], 'agent'))
    )
    const skills: unknown = JSON.parse(
      cli(binary, ['debug', 'skill'], 'skills')
    )
    summary.agent = agent
    summary.skills = skills
    if (
      !Array.isArray(skills) ||
      agent.prompt ||
      skills.some(skill => record(skill).location !== '<built-in>')
    )
      throw new Error('Custom agent or external skills remain')
    const tools = record(agent.tools)
    for (const name of [
      'bash',
      'read',
      'glob',
      'grep',
      'webfetch',
      'todowrite',
      'skill',
    ])
      if (!tools[name]) throw new Error('Normal tool missing: ' + name)
    summary.viewerDiscovery = Object.fromEntries(
      [
        ['help', ['--help']],
        ['skills', ['skills']],
      ].map(([name, args]) => {
        if (typeof name !== 'string' || !Array.isArray(args))
          throw new Error('Invalid viewer discovery command')
        return [
          name,
          { bytes: Buffer.byteLength(cli(viewer, args, 'viewer-' + name)) },
        ]
      })
    )
    summary.canary = await canary(evidence, save)
    if (process.argv.includes('--live-sol61')) {
      if (!summary.requestedModelPresentAfterRefresh)
        throw new Error('Sol 6.1 absent after native refresh; no live fallback')
      process.env.OTEL_EVAL_RUN_DIR = evidence
      summary.externalInferenceAttempted = true
      const result = await new Provider({
        config: {
          model,
          isolation: {
            opencodeBinary: binary,
            authFile,
            permission: {
              bash: { '*': 'allow', '*otel-desktop-viewer*': 'deny' },
              task: 'deny',
            },
          },
        },
      }).callApi(
        'Isolation access smoke only, not an evaluation task. Use bash to run pwd, then reply ISOLATION_SMOKE_OK. Do not read files, delegate or run other commands.',
        { vars: { task_id: 'isolation-access-smoke' } }
      )
      summary.live = {
        error: result.error,
        output: result.output,
        metadata: result.metadata,
      }
      save('live-result.json', summary.live)
      if (result.error || !result.output?.includes('ISOLATION_SMOKE_OK'))
        throw new Error('Sol 6.1 access smoke failed')
      if (
        !result.metadata.isolationRoot ||
        !result.metadata.workspace ||
        !('sessionIDs' in result.metadata)
      )
        throw new Error('Incomplete live metadata')
      const env = cleanEnvironment({ root: result.metadata.isolationRoot })
      const commands = readJson(
        path.join(result.metadata.evidence, 'commands.json')
      )
      if (!Array.isArray(commands)) throw new Error('Invalid live commands')
      summary.liveOnlyPwd =
        commands.length === 1 &&
        record(commands[0]).command === 'pwd' &&
        record(commands[0]).status === 'completed'
      for (const id of result.metadata.sessionIDs) {
        if (typeof id !== 'string') throw new Error('Invalid session ID')
        const exported = spawnSync(binary, ['export', id], {
          cwd: result.metadata.workspace,
          env,
          encoding: 'utf8',
          timeout: 120000,
          maxBuffer: 16 * 1024 * 1024,
        })
        save('live-session-' + id + '.json', exported.stdout || '')
        save('live-session-' + id + '.stderr', exported.stderr || '')
        if (exported.status !== 0) throw new Error('Live session export failed')
        const messages = record(JSON.parse(exported.stdout)).messages
        if (!Array.isArray(messages))
          throw new Error('Invalid session messages')
        const modelIDs = messages
          .map(message => record(record(message).info))
          .filter(info => info.role === 'assistant')
          .map(info => ({ providerID: info.providerID, modelID: info.modelID }))
        summary.liveExportedModelIDs = modelIDs
        if (
          !summary.liveOnlyPwd ||
          !modelIDs.some(
            model =>
              model.providerID === 'openai' && model.modelID === 'gpt-6.1-sol'
          )
        )
          throw new Error('Live smoke tool/model verification failed')
      }
    }
    summary.noPackagesInstalled =
      fs.readdirSync(path.join(clean.env.OPENCODE_CONFIG_DIR, 'node_modules'))
        .length === 0
    if (!summary.noPackagesInstalled)
      throw new Error('Unexpected package installation')
  } catch (error) {
    summary.failure = error instanceof Error ? error.message : String(error)
    process.exitCode = 1
  } finally {
    const after = originals()
    summary.originalsUnchanged =
      JSON.stringify(before) === JSON.stringify(after)
    summary.fingerprintedOriginalFiles = Object.keys(before).length
    save('original-fingerprints-after.json', after)
    save('summary.json', summary)
    if (!summary.originalsUnchanged) process.exitCode = 1
    console.log(
      JSON.stringify(
        {
          evidence,
          version: summary.version,
          requestedModelPresent: summary.requestedModelPresent,
          requestedModelPresentAfterRefresh:
            summary.requestedModelPresentAfterRefresh,
          externalInferenceAttempted: summary.externalInferenceAttempted,
          canary: summary.canary,
          originalsUnchanged: summary.originalsUnchanged,
          failure: summary.failure,
        },
        null,
        2
      )
    )
  }
}

function audit(evidence: string) {
  const output = fs.mkdtempSync(
    path.join(TEMPORARY, 'eval-isolation-integration-audit-')
  )
  fs.chmodSync(output, 0o700)
  const save = protectedWriter(output)
  const summary = record(readJson(path.join(evidence, 'summary.json')))
  const before = fingerprint(evidence),
    original = readJson(
      path.join(evidence, 'original-fingerprints-before.json')
    )
  const current = originals()
  const live = summary.live ? record(summary.live) : undefined
  const metadata = live ? record(live.metadata) : undefined
  const roots = [summary.root, metadata?.isolationRoot].filter(
    value => typeof value === 'string'
  )
  const credentials = roots.map(root =>
    path.join(root, 'data/opencode/auth.json')
  )
  const secrets = [authFile, ...credentials]
    .flatMap(file => credentialStrings(readJson(file)))
    .filter(value => value.length > 8)
  const credentialPaths = credentials.map(file => fs.realpathSync(file))
  const leakedFiles = Object.keys(before)
    .filter(
      file =>
        !('link' in before[file]) &&
        !credentialPaths.includes(fs.realpathSync(file))
    )
    .filter(file => {
      const content = fs.readFileSync(file)
      return secrets.some(secret => content.includes(Buffer.from(secret)))
    })
  const checks: Record<string, number | null> = {}
  const testTemporary = path.join(output, 'test-tmp')
  fs.mkdirSync(testTemporary, { mode: 0o700 })
  for (const [name, args] of [
    ['node-tests', ['test']],
    ['typescript', ['run', 'typecheck']],
  ] as const) {
    const result = spawnSync('npm', [...args], {
      cwd: SUITE,
      env: { ...process.env, TMPDIR: testTemporary },
      encoding: 'utf8',
      timeout: 120000,
    })
    save(name + '.txt', (result.stdout || '') + (result.stderr || ''))
    checks[name] = result.status
  }
  let sessions: unknown[] = []
  if (metadata) {
    if (typeof metadata.isolationRoot !== 'string')
      throw new Error('Invalid live isolation root')
    const source = path.join(
        metadata.isolationRoot,
        'data/opencode/opencode.db'
      ),
      snapshot = path.join(output, 'session-snapshot.db')
    for (const suffix of ['', '-wal', '-shm'])
      if (fs.existsSync(source + suffix)) {
        fs.copyFileSync(
          source + suffix,
          snapshot + suffix,
          fs.constants.COPYFILE_EXCL
        )
        fs.chmodSync(snapshot + suffix, 0o600)
      }
    const db = new DatabaseSync(snapshot, { readOnly: true })
    try {
      sessions = db
        .prepare('SELECT id FROM session')
        .all()
        .map(row => row.id)
    } finally {
      db.close()
    }
  }
  const after = fingerprint(evidence)
  const sessionIDs = metadata?.sessionIDs
  const result = {
    evidence,
    auditArtifacts: output,
    checks,
    leakedFiles,
    originalsStillUnchanged:
      JSON.stringify(original) === JSON.stringify(current),
    retainedVerificationUnchanged:
      JSON.stringify(before) === JSON.stringify(after),
    changedVerificationFiles: [
      ...new Set([...Object.keys(before), ...Object.keys(after)]),
    ].filter(
      file => JSON.stringify(before[file]) !== JSON.stringify(after[file])
    ),
    credentialCopiesPrivate: credentials.every(
      file => (fs.statSync(file).mode & 0o777) === 0o600
    ),
    noPackagesInstalled: roots.every(
      root =>
        fs.readdirSync(path.join(root, 'config/opencode/node_modules'))
          .length === 0
    ),
    liveSessionIDs: sessions,
    onlyFreshLiveSession:
      !live ||
      (sessions.length === 1 &&
        Array.isArray(sessionIDs) &&
        sessions[0] === sessionIDs[0]),
    priorFilesFingerprinted: Object.keys(record(original)).length,
    verificationFilesScanned: Object.keys(before).length,
  }
  save('audit.json', result)
  if (
    Object.values(checks).some(code => code !== 0) ||
    leakedFiles.length ||
    !result.originalsStillUnchanged ||
    !result.retainedVerificationUnchanged ||
    !result.credentialCopiesPrivate ||
    !result.noPackagesInstalled ||
    !result.onlyFreshLiveSession
  )
    process.exitCode = 1
  console.log(JSON.stringify(result, null, 2))
}

if (process.argv.includes('--audit')) {
  try {
    const evidence = process.argv[process.argv.indexOf('--audit') + 1]
    if (!evidence) throw new Error('Missing audit evidence')
    audit(evidence)
  } catch {
    console.error('Isolation audit failed; inspect new protected artifacts')
    process.exitCode = 1
  }
} else
  main().catch(() => {
    console.error(
      'Isolation verification failed; inspect new protected artifacts'
    )
    process.exitCode = 1
  })
