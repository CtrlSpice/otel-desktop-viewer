import fs from 'node:fs'
import path from 'node:path'
import { createHash } from 'node:crypto'
import { execFileSync } from 'node:child_process'
import { V2_BINARY } from './opencode-v2-provider.mjs'

// Read-only executable inspection; help commands run with a fresh private profile.
process.umask(0o077)
const root = fs.mkdtempSync('/Users/moya/.local/share/otel-desktop-viewer/evals/opencode-v2-telemetry-discovery-')
const save = (name, value) => fs.writeFileSync(path.join(root, name), typeof value === 'string' ? value : JSON.stringify(value, null, 2) + '\n', { flag: 'wx', mode: 0o600 })
const binary = fs.readFileSync(V2_BINARY)
const env = { PATH: '/opt/homebrew/bin:/usr/bin:/bin', OPENCODE_CONFIG_PROJECT_DISABLE: '1', LANG: 'en_US.UTF-8' }
for (const key of ['HOME', 'XDG_CONFIG_HOME', 'XDG_DATA_HOME', 'XDG_STATE_HOME', 'XDG_CACHE_HOME', 'TMPDIR']) {
  env[key] = path.join(root, key); fs.mkdirSync(env[key], { mode: 0o700 })
}
env.OPENCODE_TEST_HOME = env.HOME
for (const [file, args] of [['version.txt', ['--version']], ['help.txt', ['--help']], ['serve-help.txt', ['serve', '--help']]])
  save(file, execFileSync(V2_BINARY, args, { cwd: root, env, encoding: 'utf8', timeout: 10000 }))
const marker = Buffer.from('function xZ(e={endpoint:process.env.OTEL_EXPORTER_OTLP_ENDPOINT')
const offset = binary.indexOf(marker)
if (offset < 0) throw new Error('Inspected observability implementation not found')
// Preserve the literal embedded source bytes, including the native logger and
// trace setup immediately preceding xZ. This is evidence, never executable code.
save('observability-embedded-source.txt', binary.subarray(offset - 2600, offset + 850).toString('utf8'))
const controls = [...new Set(binary.toString('latin1').match(/OPENCODE_[A-Z_]+/g))]
  .filter(name => /TRAC|SPAN|LOG|DEBUG|OTEL|METRIC|TELEMETRY/.test(name)).sort()
save('capabilities.json', { binary: V2_BINARY, sha256: createHash('sha256').update(binary).digest('hex'), offset, controls,
  documentation: ['https://opencode.ai/v2/docs/config/', 'https://opencode.ai/v2/docs/cli/', 'https://opencode.ai/v2/docs/troubleshooting/'],
  findings: {
    endpoint: 'OTEL_EXPORTER_OTLP_ENDPOINT is read directly by CLI observability setup',
    headers: 'OTEL_EXPORTER_OTLP_HEADERS is supported; no headers are needed for the local observer',
    logs: 'Native Effect logger exports /v1/logs',
    traces: 'Native @effect/opentelemetry layer installs BatchSpanProcessor and exports /v1/traces',
    metrics: 'No metric reader/exporter is wired in this observability setup; generic library metric code being present is not enablement',
    level: 'OPENCODE_LOG_LEVEL controls MinimumLogLevel only; no exposed semantic span selection or minimum trace level found',
    directTrace: 'OPENCODE_DIRECT_TRACE writes direct-client JSONL diagnostic records; it is not OTLP span selection',
    propagation: 'HTTP middleware parses W3C traceparent; session-runner propagation must be measured separately',
  },
})
console.log(JSON.stringify({ root, controls, binarySHA256: createHash('sha256').update(binary).digest('hex') }))
