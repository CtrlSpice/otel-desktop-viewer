import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import http from 'node:http'
import { once } from 'node:events'
import { createHash } from 'node:crypto'
import { execFileSync } from 'node:child_process'

// Preparation only. --probe uses the real pinned Promptfoo provider against a
// synthetic loopback OpenCode HTTP server. It never starts OpenCode or a model.
const suite = import.meta.dirname
const args = process.argv.slice(2)
if (args.some(arg => arg !== '--probe')) throw new Error('Usage: node opencode-native-preflight.mjs [--probe]')
const parent = fs.realpathSync(process.env.OTEL_EVAL_OPENCODE_PREP_PARENT ?? '/Users/moya/.local/share/otel-desktop-viewer/evals')
for (let dir = parent; ; dir = path.dirname(dir)) {
  if (fs.existsSync(path.join(dir, '.git'))) throw new Error('Preparation evidence must be outside repositories')
  if (dir === path.dirname(dir)) break
}
process.umask(0o077)
const root = fs.mkdtempSync(path.join(parent, 'opencode-native-prep-'))
const save = (name, value) => fs.writeFileSync(path.join(root, name), JSON.stringify(value, null, 2) + '\n', { flag: 'wx', mode: 0o600 })
const read = file => JSON.parse(fs.readFileSync(file, 'utf8'))
const hash = file => createHash('sha256').update(fs.readFileSync(file)).digest('hex')
const source = path.join(parent, 'native-otel-six-gLWzRd')
const protectedFiles = [
  ...['tasks.json', 'rubrics.json', 'prompt.txt', 'grade.ts', 'grading-evidence.ts'].map(file => path.join(suite, file)),
  ...['promptfoo.json', 'promptfoo-rows.jsonl', 'run-plan.json'].map(file => path.join(source, file)),
].filter(file => fs.existsSync(file))
const before = Object.fromEntries(protectedFiles.map(file => [file, hash(file)]))
save('source-fingerprints.json', before)

const promptfoo = read(path.join(suite, 'node_modules/promptfoo/package.json'))
const sdk = read(path.join(suite, 'node_modules/@opencode-ai/sdk/package.json'))
assert.equal(promptfoo.version, '0.124.1', 'Reinspect provider implementation after changing Promptfoo')
assert.equal(sdk.version, '1.18.35', 'Reinspect SDK contract after changing SDK')
const binary = process.env.OTEL_EVAL_OPENCODE_BINARY ?? '/opt/homebrew/bin/opencode'
const authFile = process.env.OTEL_EVAL_OPENCODE_AUTH_FILE ?? '/Users/moya/.local/share/opencode/auth.json'
// Report credential presence/type only. Never copy or serialize credential values.
let authentication = { present: false, inferenceAccess: 'not tested' }
if (fs.existsSync(authFile)) {
  const mode = fs.statSync(authFile).mode & 0o777
  if (mode & 0o077) throw new Error('Selected auth file must be private')
  const auth = read(authFile)
  authentication = { present: Boolean(auth.openai), type: auth.openai?.type, mode: mode.toString(8), inferenceAccess: 'not tested' }
}

// Do not inherit the current agent's profile, credentials, repository selectors,
// tracing exporter, or extensions into the probe's Promptfoo module graph.
const clean = {
  PATH: '/opt/homebrew/bin:/usr/bin:/bin:/usr/sbin:/sbin',
  HOME: path.join(root, 'home'), XDG_CONFIG_HOME: path.join(root, 'config'),
  XDG_DATA_HOME: path.join(root, 'data'), XDG_STATE_HOME: path.join(root, 'state'),
  XDG_CACHE_HOME: path.join(root, 'cache'), TMPDIR: path.join(root, 'tmp'),
  LANG: 'en_US.UTF-8', SHELL: '/bin/sh',
  PROMPTFOO_CONFIG_DIR: path.join(root, 'promptfoo-state'),
  PROMPTFOO_DISABLE_TELEMETRY: '1', PROMPTFOO_DISABLE_UPDATE: '1',
  PROMPTFOO_OTEL_ENABLED: 'false',
}
for (const key of ['HOME', 'XDG_CONFIG_HOME', 'XDG_DATA_HOME', 'XDG_STATE_HOME', 'XDG_CACHE_HOME', 'TMPDIR', 'PROMPTFOO_CONFIG_DIR'])
  fs.mkdirSync(clean[key], { recursive: true, mode: 0o700 })
const cliVersion = execFileSync(binary, ['--version'], { env: clean, cwd: root, timeout: 10000, encoding: 'utf8' }).trim()
const task = read(path.join(suite, 'tasks.json')).find(task => task.id === 'checkout-failure')
assert.ok(task)
const blockers = [
  ...(cliVersion.startsWith('2.') ? [] : [`Installed CLI ${cliVersion} is not OpenCode V2`]),
  'Promptfoo 0.124.1 uses @opencode-ai/sdk/v2 legacy /session APIs, not @opencode/client and /api/session',
  'Existing clean-environment.ts and native-telemetry-plugin.mjs are pinned to CLI 1.18.10; V2 isolation and hooks need verification',
  'Native provider lacks complete retained tool/session evidence and does not map assistant info.error to ProviderResponse.error',
  'grading-evidence.ts accepts Pi/Claude identities only; an OpenCode-specific evidence adapter is needed',
  'Selected OpenAI login presence does not establish access to either model in an isolated V2 profile',
]
save('compatibility.json', {
  status: 'blocked', paidRequests: 0, cli: { binary, version: cliVersion },
  promptfoo: promptfoo.version, sdk: { name: sdk.name, version: sdk.version, entrypoint: '@opencode-ai/sdk/v2', productGeneration: 1 },
  authentication, blockers,
})
save('one-question-plan.json', {
  status: 'blocked; design only, not an executable evaluation config',
  task, promptFile: path.join(suite, 'prompt.txt'), rubricFile: path.join(suite, 'rubrics.json'),
  investigator: { providerID: 'opencode:sdk', model: 'openai/gpt-6.1-sol', freshSession: true, freshProfile: true },
  grader: { providerID: 'opencode:sdk', model: 'openai/gpt-6.1-sol', freshSession: true, freshProfile: true, tools: false },
  scheduling: { tests: 1, providers: 1, repeat: 1, maxConcurrency: 1, cache: false },
  assertion: { type: 'llm-rubric', threshold: 1, scores: [0, 0.5, 1], source: 'approved checkout-failure rubric plus shared grading instructions and retained tool evidence' },
  outputPath: ['promptfoo.json', 'promptfoo-rows.jsonl'], writeLatestResults: true,
  observer: { backend: 'http://127.0.0.1:56008', otlpHTTP: 'http://127.0.0.1:56010', preview: 'http://127.0.0.1:56007' },
  fixture: 'New owned viewer and manifest-verified dataset, separate from observer and original six-run evidence',
  sourceEvidence: source,
  requiredEvidence: ['native full v3 export', 'native incremental rows', 'separate investigator/grader responses', 'raw session/tool events', 'prompt and answer', 'trace parent and session identities', 'OTLP receipts', 'owned-process shutdown'],
})

async function probe() {
  for (const key of Object.keys(process.env)) delete process.env[key]
  Object.assign(process.env, clean)
  const { evaluate, loadApiProvider } = await import('promptfoo')
  const { SHARED_INSTRUCTIONS } = await import('./grade.ts')
  const rubric = read(path.join(suite, 'rubrics.json'))[task.id]
  const requests = []
  const sessionIDs = []
  const answer = 'Synthetic contract answer only. No telemetry investigation was performed.'
  const server = http.createServer(async (req, res) => {
    try {
      let raw = ''
      for await (const chunk of req) raw += chunk
      const body = raw ? JSON.parse(raw) : undefined
      const route = new URL(req.url, 'http://127.0.0.1').pathname
      requests.push({ method: req.method, route, body })
      res.setHeader('Content-Type', 'application/json')
      if (req.method === 'POST' && route === '/session') {
        const id = 'ses_contract_' + (sessionIDs.length + 1)
        sessionIDs.push(id)
        res.end(JSON.stringify({ id }))
      } else if (req.method === 'POST' && /^\/session\/[^/]+\/message$/.test(route)) {
        const sessionID = route.split('/')[2]
        const model = body.model.modelID
        const failed = model === 'contract-error'
        const text = model === 'contract-grader'
          ? JSON.stringify({ pass: false, score: 0.5, reason: 'Synthetic partial verdict; validates transport only' }) : answer
        res.end(JSON.stringify({
          info: { id: 'msg_' + sessionID, sessionID, role: 'assistant', parentID: 'msg_user',
            modelID: model, providerID: 'openai', finish: 'stop',
            tokens: { input: 10, output: 5, reasoning: 0, cache: { read: 0, write: 0 } },
            ...(failed ? { error: { name: 'APIError', data: { message: 'Synthetic model access error' } } } : {}),
          },
          parts: [{ id: 'part_' + sessionID, sessionID, messageID: 'msg_' + sessionID, type: 'text', text }],
        }))
      } else if (req.method === 'DELETE' && /^\/session\/[^/]+$/.test(route)) res.end('true')
      else { res.statusCode = 404; res.end(JSON.stringify({ error: 'Unexpected probe route' })) }
    } catch { res.statusCode = 500; res.end('{}') }
  })
  server.listen(0, '127.0.0.1')
  await once(server, 'listening')
  const baseUrl = 'http://127.0.0.1:' + server.address().port
  const provider = model => ({
    id: 'opencode:sdk', label: model,
    config: { provider_id: 'openai', model, baseUrl, tools: { '*': false }, persist_sessions: false },
  })
  let errorProvider
  try {
    const config = {
      description: 'SYNTHETIC OpenCode native-provider contract probe; not investigation evidence',
      prompts: ['file://' + path.join(suite, 'prompt.txt')],
      providers: [provider('contract-investigator')],
      defaultTest: {
        vars: { binary: './bin/otel-desktop-viewer', endpoint: baseUrl, start: 'synthetic', end: 'synthetic' },
        options: { provider: provider('contract-grader') },
        assert: [{ type: 'llm-rubric', threshold: 1, value: `${SHARED_INSTRUCTIONS}\nUse JSON pass, score, reason. Fail=0, Partial=0.5, Pass=1.\n${JSON.stringify({ question: task.goal, ...rubric })}` }],
      },
      tests: [{ description: task.id, vars: { task_id: task.id, goal: task.goal } }],
      evaluateOptions: { repeat: 1, maxConcurrency: 1, cache: false },
      tracing: { enabled: false },
      outputPath: [path.join(root, 'synthetic-promptfoo.json'), path.join(root, 'synthetic-rows.jsonl')],
      writeLatestResults: true,
    }
    save('synthetic-config.json', config)
    await evaluate(config, { cache: false, repeat: 1, maxConcurrency: 1, share: false })
    const exported = read(path.join(root, 'synthetic-promptfoo.json'))
    assert.ok(exported.evalId)
    assert.ok(exported.config)
    assert.equal(exported.results.results.length, 1)
    const row = exported.results.results[0]
    assert.equal(row.response.output, answer)
    assert.equal(row.score, 0.5)
    assert.equal(row.success, false)
    assert.equal(row.gradingResult.componentResults[0].assertion.type, 'llm-rubric')
    const calls = requests.filter(request => request.route.endsWith('/message'))
    assert.equal(calls.length, 2)
    assert.notEqual(calls[0].route, calls[1].route, 'Grading requires a separate native session')
    assert.equal(calls[0].body.parts[0].text.split('\n')[0], task.goal)
    assert.ok(!JSON.stringify(calls[0].body).includes(rubric.reference), 'Rubric must not enter investigator prompt')
    // This is deliberately a diagnostic for the installed provider's defect,
    // not a workaround that reclassifies or rewrites a native result row.
    errorProvider = await loadApiProvider('opencode:sdk', { options: { config: provider('contract-error').config } })
    const response = await errorProvider.callApi('Synthetic assistant error-envelope probe')
    save('synthetic-error-response.json', response)
    assert.ok(JSON.parse(response.raw).data.info.error)
    assert.equal(response.error, undefined, 'Provider behavior changed; reassess error blocker')
    assert.equal(response.output, answer)
    save('probe.json', {
      status: 'contract confirmed; V2 remains blocked', paidRequests: 0, realOpenCodeSessions: 0,
      nativeResultRows: 1, syntheticInvestigatorCalls: 1, syntheticGraderCalls: 1, errorEnvelopeProbes: 1,
      separateSessions: true, partialScorePreserved: true, nativeFullExport: true,
      usesLegacyRoutes: requests.every(request => request.route.startsWith('/session')),
      assistantErrorSilentlyOmitted: true,
    })
  } finally {
    await errorProvider?.cleanup?.()
    save('synthetic-http-requests.json', requests)
    server.closeAllConnections()
    await new Promise(resolve => server.close(resolve))
  }
}

try {
  if (args.includes('--probe')) await probe()
} finally {
  const after = Object.fromEntries(protectedFiles.map(file => [file, hash(file)]))
  assert.deepEqual(after, before, 'Protected source changed while preparing; investigate concurrent edits')
  save('source-preservation.json', { unchanged: true, files: after })
  console.log(JSON.stringify({ root, status: 'V2 blocked', paidRequests: 0, probeRequested: args.includes('--probe') }))
}
