import fs from 'node:fs'
import path from 'node:path'
import { spawn, execFileSync } from 'node:child_process'
import { once } from 'node:events'

const binary = '/Users/moya/Library/Application Support/ai.opencode.desktop/cli/2.0.29-arm64/opencode-cli'
const parent = '/Users/moya/.local/share/otel-desktop-viewer/evals'
process.umask(0o077)
const root = fs.mkdtempSync(path.join(parent, 'opencode-v2-discovery-'))
const env = { PATH: '/opt/homebrew/bin:/usr/bin:/bin', SHELL: '/bin/sh', LANG: 'en_US.UTF-8' }
for (const [key, name] of Object.entries({ HOME: 'home', XDG_CONFIG_HOME: 'config', XDG_DATA_HOME: 'data', XDG_STATE_HOME: 'state', XDG_CACHE_HOME: 'cache', TMPDIR: 'tmp' })) {
  env[key] = path.join(root, name)
  fs.mkdirSync(env[key], { mode: 0o700 })
}
const workspace = path.join(root, 'workspace')
fs.mkdirSync(workspace, { mode: 0o700 })
fs.mkdirSync(path.join(env.XDG_CONFIG_HOME, 'opencode'), { mode: 0o700 })
fs.writeFileSync(path.join(env.XDG_CONFIG_HOME, 'opencode/opencode.json'), JSON.stringify({
  $schema: 'https://opencode.ai/config.json', update: 'disable', snapshots: false,
  permissions: [{ action: '*', resource: '*', effect: 'deny' }],
}), { mode: 0o600 })
const version = execFileSync(binary, ['--version'], { env, cwd: workspace, encoding: 'utf8' }).trim()
if (version !== 'opencode v2.0.29') throw new Error('Unexpected V2 binary')
const child = spawn(binary, ['serve', '--hostname', '127.0.0.1', '--port', '0'], { env, cwd: workspace, detached: true, stdio: ['ignore', 'pipe', 'pipe'] })
const closed = once(child, 'close')
let stdout = '', stderr = ''
child.stdout.on('data', chunk => { stdout += chunk; fs.appendFileSync(path.join(root, 'stdout.log'), chunk) })
child.stderr.on('data', chunk => { stderr += chunk; fs.appendFileSync(path.join(root, 'stderr.log'), chunk) })
try {
  const url = await new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error('No server URL; see ' + root)), 30000)
    child.stdout.on('data', () => { const found = stdout.match(/https?:\/\/127\.0\.0\.1:\d+/)?.[0]; if (found && /server password\s+\S+/.test(stdout)) { clearTimeout(timer); resolve(found) } })
    closed.then(() => { clearTimeout(timer); reject(new Error('Server exited; see ' + root)) })
  })
  for (const route of ['/api/info', '/openapi.json', '/api/config', '/api/agent', '/api/plugin', '/api/model', '/api/integration']) {
    const token = stdout.match(/server password\s+(\S+)/)?.[1]
    const response = await fetch(url + route, { headers: { Authorization: 'Basic ' + Buffer.from('opencode:' + token).toString('base64') }, signal: AbortSignal.timeout(30000) })
    fs.writeFileSync(path.join(root, route.replaceAll('/', '_') + '.json'), await response.text())
    console.log(JSON.stringify({ route, status: response.status }))
  }
} finally {
  if (child.exitCode === null) process.kill(-child.pid, 'SIGTERM')
  const timer = setTimeout(() => { try { process.kill(-child.pid, 'SIGKILL') } catch {} }, 10000)
  const exit = await closed
  clearTimeout(timer)
  fs.writeFileSync(path.join(root, 'owned-shutdown.json'), JSON.stringify({ pid: child.pid, exit }))
  console.log(JSON.stringify({ root, version, paidRequests: 0 }))
}
