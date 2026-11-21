import fs from 'node:fs'
import path from 'node:path'
import { homedir, tmpdir, userInfo } from 'node:os'
import { execFileSync, spawn } from 'node:child_process'
import { once } from 'node:events'
import { pathToFileURL } from 'node:url'
import Provider, { connectRunModels, V2_BINARY, V2_VERSION } from './opencode-v2-provider.mjs'
import { fullRunPlan } from './opencode-v2-run-plan.mjs'
import { stopOwnedGroup, runCancellation } from './run-cancellation.ts'
import { setTimeout as delay } from 'node:timers/promises'
import { nativeTelemetryReceiver } from './opencode-v2-telemetry.mjs'
import http from 'node:http'

// Session.wait sends no headers until the agent finishes. Bound this loopback
// request by the existing investigation deadline, not fetch's header timeout.
export function waitForIdle(url, headers, signal) {
  return new Promise((resolve, reject) => {
    const request = http.request(url, { method: 'POST', headers, signal }, response => {
      response.on('error', reject)
      response.on('end', () => response.statusCode === 204 ? resolve() : reject(Error('Session.wait HTTP ' + response.statusCode)))
      response.resume()
    })
    request.on('error', reject)
    request.end()
  })
}

export function hostApi(url, headers, ownerSignal) {
  return async (route, method = 'GET', body, traceparent, requestSignal) => {
    const requestHeaders = { ...headers, ...(traceparent ? { traceparent } : {}) }
    // Attempts supply their combined signal; teardown supplies its own deadline.
    const signal = requestSignal ?? AbortSignal.any([ownerSignal, AbortSignal.timeout(90000)])
    if (method === 'POST' && /^\/api\/experimental\/session\/ses_[A-Za-z0-9]+\/wait$/.test(route))
      return waitForIdle(url + route, requestHeaders, signal)
    const response = await fetch(url + route, { method, headers: requestHeaders, redirect: 'error', signal,
      ...(body ? { body: JSON.stringify(body) } : {}) })
    if (!response.ok) throw Error('OpenCode ' + method + ' ' + route + ': HTTP ' + response.status)
    const raw = await response.text()
    return raw ? JSON.parse(raw) : undefined
  }
}

// A connection check uses no viewer fixture and grants no tools. It is not an
// eval result. Credentials stay in the owned service's private profile.
export async function checkConnections({ root, catalogue, files, runtime, probe, refs = fullRunPlan().models, evaluateOnHost, telemetryForward, signal, scriptedModelEndpoint }) {
  if (scriptedModelEndpoint && (!/^http:\/\/127\.0\.0\.1:\d+\/v1$/.test(scriptedModelEndpoint) || refs.length || files || !evaluateOnHost))
    throw Error('Scripted shared host requires a loopback model with no memberships')
  process.umask(0o077)
  const work = fs.mkdtempSync(path.join(root, 'connections-'))
  const plugin = path.join(runtime, 'claude-plugin/node_modules/opencode-with-claude')
  const claudeBinary = path.join(runtime, 'claude-plugin/node_modules/@anthropic-ai/claude-code-darwin-arm64/claude')
  // The CLI's macOS subscription login uses the user's home to find Keychain.
  // Scope HOME to the CLI subprocess rather than changing OpenCode's profile.
  const cli = path.join(work, 'claude')
  const quote = s => "'" + s.replaceAll("'", "'\\''") + "'"
  fs.writeFileSync(cli, `#!/bin/sh\nexport HOME=${quote(homedir())}\nexec ${quote(claudeBinary)} "$@"\n`, { mode: 0o700 })
  const claudeProfile = path.join(runtime, 'claude-profile')
  const env = { PATH: '/opt/homebrew/bin:/usr/bin:/bin', USER: userInfo().username, SHELL: '/bin/sh',
    OPENCODE_MODELS_PATH: fs.realpathSync(catalogue), OPENCODE_DISABLE_MODELS_FETCH: '1',
    OPENCODE_CONFIG_PROJECT_DISABLE: '1', OPENCODE_FILEWATCHER_DISABLE: '1',
    MERIDIAN_CLAUDE_PATH: cli, CLAUDE_CONFIG_DIR: claudeProfile, CLAUDE_PROXY_HOST: '127.0.0.1', CLAUDE_PROXY_PORT: '0',
    MERIDIAN_PROFILES: JSON.stringify([{ id: 'viewer-eval', claudeConfigDir: claudeProfile }]),
    MERIDIAN_DEFAULT_PROFILE: 'viewer-eval', MERIDIAN_PASSTHROUGH: 'true', MERIDIAN_MAX_CONCURRENT: '1' }
  for (const key of ['HOME', 'XDG_CONFIG_HOME', 'XDG_DATA_HOME', 'XDG_CACHE_HOME', 'XDG_STATE_HOME', 'TMPDIR']) {
    env[key] = path.join(work, key); fs.mkdirSync(env[key], { mode: 0o700 })
  }
  env.OPENCODE_TEST_HOME = env.HOME
  const permissions = [{ action: '*', resource: '*', effect: 'deny' }]
  const config = path.join(env.XDG_CONFIG_HOME, 'opencode')
  fs.mkdirSync(config)
  const entry = path.join(work, 'claude-plugin')
  fs.mkdirSync(entry)
  fs.writeFileSync(path.join(entry, 'package.json'), JSON.stringify({ name: 'opencode-with-claude', private: true, type: 'module', exports: { '.': './index.mjs' } }))
  fs.writeFileSync(path.join(entry, 'index.mjs'), `export { default } from ${JSON.stringify(pathToFileURL(path.join(plugin, 'dist/index.js')).href)}\n`)
  const plugins = ['-opencode.browser', ...(scriptedModelEndpoint ? [] : [pathToFileURL(entry).href])]
  if (scriptedModelEndpoint) env.OPENAI_API_KEY = 'synthetic-only'
  let telemetry
  if (evaluateOnHost) {
    const policy = path.join(work, 'policy')
    fs.mkdirSync(policy)
    for (const [src, dest] of [['opencode-v2-policy.mjs','opencode-v2-policy.mjs'],['opencode-v2-shared-policy.mjs','index.mjs'],['opencode-v2-workspace-guard.mjs','opencode-v2-workspace-guard.mjs']])
      fs.copyFileSync(path.join(import.meta.dirname, src), path.join(policy, dest))
    fs.writeFileSync(path.join(policy, 'package.json'), JSON.stringify({ name: 'viewer-eval-policy', private: true, type: 'module', exports: { '.': './index.mjs' } }))
    plugins.push(pathToFileURL(policy).href)
    env.OTEL_EVAL_V2_SESSION_REGISTRY = path.join(work, 'sessions')
    env.OTEL_EVAL_SHELL_PROTECTED = JSON.stringify([...new Set([homedir(), tmpdir(), root, runtime, path.resolve(import.meta.dirname, '../..'),
      ...Object.values(files ?? {}).map(source => typeof source === 'string' ? source : source?.file),
      process.env.OTEL_EVAL_OPENCODE_AUTH_FILE, process.env.OTEL_EVAL_PI_AUTH_FILE, process.env.OTEL_EVAL_CLAUDE_CONFIG_DIR,
    ].filter(Boolean).map(file => fs.realpathSync(file)))])
    fs.mkdirSync(env.OTEL_EVAL_V2_SESSION_REGISTRY)
    env.OTEL_EVAL_V2_POLICY_LOG = path.join(work, 'policy.jsonl')
    env.OTEL_EVAL_CURL_BIN = path.join(work, 'viewer-http-bin')
    fs.mkdirSync(env.OTEL_EVAL_CURL_BIN)
    fs.copyFileSync(path.join(import.meta.dirname, 'opencode-v2-curl.mjs'), path.join(policy, 'opencode-v2-curl.mjs'))
    fs.writeFileSync(path.join(env.OTEL_EVAL_CURL_BIN, 'curl'), `#!/bin/sh\nexec ${quote(process.execPath)} ${quote(path.join(policy, 'opencode-v2-curl.mjs'))} "$@"\n`, { mode: 0o500 })
    fs.writeFileSync(path.join(policy, 'fixture-shell'), '#!/bin/sh\nexec /usr/bin/sandbox-exec -p "${OTEL_EVAL_SHELL_PROFILE:?}" /bin/sh "$@"\n', { mode: 0o500 })
    telemetry = await nativeTelemetryReceiver(root, telemetryForward)
    env.OTEL_EXPORTER_OTLP_ENDPOINT = telemetry.endpoint
    env.OPENCODE_CLIENT = 'viewer-eval-v2'
  }
  fs.writeFileSync(path.join(config, 'opencode.json'), JSON.stringify({ update: 'disable', warming: false, snapshots: false,
    permissions, agents: { title: { disabled: true }, build: { permissions } },
    plugins,
    providers: { anthropic: { settings: { baseURL: 'http://127.0.0.1:3456/v1', apiKey: 'dummy' } },
      openai: { settings: { transport: 'http', ...(scriptedModelEndpoint ? { baseURL: scriptedModelEndpoint, apiKey: 'synthetic-only' } : {}) } } } }))
  const owner = runCancellation(signal)
  const child = spawn(V2_BINARY, ['serve', '--hostname', '127.0.0.1', '--port', '0'], { cwd: work, env, detached: true, stdio: ['ignore', 'pipe', 'ignore'] })
  const closed = once(child, 'close')
  const results = []
  try {
    let stdout = ''
    const startSignal = AbortSignal.any([owner.signal, AbortSignal.timeout(30000)])
    while (!/server password\s+\S+\r?\n/.test(stdout) || !/http:\/\/127\.0\.0\.1:\d+/.test(stdout)) {
      const [chunk] = await Promise.race([once(child.stdout, 'data', { signal: startSignal }), closed.then(() => { throw Error('Eval host exited') })])
      stdout += chunk
    }
    const url = stdout.match(/http:\/\/127\.0\.0\.1:\d+/)[0]
    const headers = { Authorization: 'Basic ' + Buffer.from('opencode:' + stdout.match(/server password\s+(\S+)/)[1]).toString('base64'), 'Content-Type': 'application/json' }
    const api = hostApi(url, headers, owner.signal)
    if ((await api('/api/info')).version !== V2_VERSION) throw Error('Incorrect eval runtime version')
    const selected = await connectRunModels(api, refs, files)
    if (evaluateOnHost) {
      results.push(...selected)
      const checkPolicy = async location => {
        const route = '/api/plugin' + (location ? '?location[directory]=' + encodeURIComponent(location) : '')
        const deadline = Date.now() + 15000
        for (;;) {
          const response = await api(route)
          if (location && response.location?.directory !== location) throw Error('Policy location mismatch')
          const list = response.data
          if (list.some(p => p.id === 'viewer-eval-policy' && p.state.status === 'active')) return
          if (Date.now() >= deadline) throw Error('Workspace protection did not activate')
          await delay(100)
        }
      }
      await checkPolicy()
      return await evaluateOnHost({ root, work, api, checkPolicy, telemetry, registry: env.OTEL_EVAL_V2_SESSION_REGISTRY, signal: owner.signal }, selected)
    }
    for (const { providerID, id, enabled } of selected) {
      const result = { model: providerID + '/' + id, enabled, answered: false }
      results.push(result)
      try {
        if (!enabled) throw Error('Model unavailable in eval host')
        if (!probe) continue
        if (providerID === 'anthropic') {
          let loggedIn = false
          try { const status = JSON.parse(execFileSync(cli, ['auth', 'status'], { env, stdio: ['ignore', 'pipe', 'ignore'], timeout: 10000 }))
            loggedIn = status.loggedIn && status.authMethod === 'claude.ai' } catch {}
          if (!loggedIn) throw Error('Eval Claude subscription sign-in pending')
        }
        const provider = new Provider({ config: { providerID, model: id } })
        const session = (await provider.createSession(api, work, permissions, 'Membership connection check')).data.id
        await api('/api/session/' + session + '/prompt', 'POST', { text: 'Reply OK. Do not use tools.' })
        await api('/api/experimental/session/' + session + '/wait', 'POST')
        const history = (await api('/api/session/' + session + '/context')).data
        const assistant = history.findLast(m => m.type === 'assistant')
        const error = history.find(m => m.error)?.error
        if (error) throw Error([error.type, error.status].filter(Boolean).join(' ') || 'Provider error')
        result.answered = assistant?.model?.providerID === providerID && assistant.model.id === id &&
          assistant.finish === 'stop' && assistant.content.some(c => c.type === 'text' && c.text.trim())
      } catch (error) { result.error = error.message }
      finally { console.log(JSON.stringify(result)) }
    }
  } finally {
    await stopOwnedGroup(child.pid, child); await closed; owner.dispose()
    if (telemetry) await telemetry.close()
    fs.writeFileSync(path.join(work, 'connection-results.json'), JSON.stringify(results, null, 2))
  }
  return results
}

if (process.argv[1] && pathToFileURL(path.resolve(process.argv[1])).href === import.meta.url) {
  const env = process.env
  const results = await checkConnections({ root: fs.realpathSync(env.OTEL_EVAL_CONNECTION_DIR), catalogue: env.OPENCODE_MODELS_PATH,
    files: { openai: env.OTEL_EVAL_OPENCODE_AUTH_FILE, 'opencode-go': env.OTEL_EVAL_GO_AUTH_FILE, 'kimi-code-plan-global': env.OTEL_EVAL_KIMI_AUTH_FILE },
    runtime: path.join(homedir(), '.local/share/otel-desktop-viewer/evals/runtime'), probe: process.argv.includes('--probe') })
  if (results.some(r => !r.enabled || (process.argv.includes('--probe') && !r.answered))) process.exitCode = 1
}
