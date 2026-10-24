import test from 'node:test'
import assert from 'node:assert/strict'
import { permissionDenial } from './permission-stop.ts'

const viewer = '/synthetic/run/scratch/bin/otel-desktop-viewer'
const directory = '/synthetic/run/scratch/bin'
const reason =
  'The user has specified a rule which prevents you from using this specific tool call. Here are some of the relevant rules ' +
  JSON.stringify([
    { permission: 'external_directory', action: 'deny', pattern: '*' },
  ])
function event(
  command: string,
  tool = 'bash',
  status = 'error',
  error = reason
) {
  return JSON.stringify({
    type: 'tool_use',
    sessionID: 'ses_synthetic',
    part: {
      type: 'tool',
      callID: 'call_synthetic',
      tool,
      state: {
        status,
        input: { command, workdir: directory },
        error,
        output: reason,
      },
    },
  })
}

test('native rule denials stop supplied-viewer help and existing-viewer clients only', () => {
  for (const command of [
    './otel-desktop-viewer --help',
    './otel-desktop-viewer -h',
    './otel-desktop-viewer --version',
    `"${viewer}" query 'SELECT 1'`,
    `${viewer} help query`,
    'otel-desktop-viewer skills',
    'otel-desktop-viewer services',
    'otel-desktop-viewer attributes keys',
    'otel-desktop-viewer traces',
    'otel-desktop-viewer trace a',
    'otel-desktop-viewer span a',
    'otel-desktop-viewer logs',
    'otel-desktop-viewer log a',
    'otel-desktop-viewer metrics',
    'otel-desktop-viewer metric a',
    'otel-desktop-viewer export traces a',
    'otel-desktop-viewer import a.json',
    `env LANG=C ${viewer} skills`,
    `pwd && ${viewer} --help`,
    `cd /synthetic/run/scratch && ./bin/otel-desktop-viewer --help`,
    `> help.txt ${viewer} --help`,
    `${viewer} > help.txt --help`,
    `${viewer} >> help.txt query 'SELECT 1'`,
    `2> errors.txt ${viewer} --help`,
    `2>&1 ${viewer} query 'SELECT 1'`,
    `cd /synthetic/run/scratch && 2> errors.txt ./bin/otel-desktop-viewer query 'SELECT 1'`,
  ])
    assert.equal(
      permissionDenial(event(command), directory, [viewer])?.stopsRun,
      true,
      command
    )
  for (const command of [
    'cat rubric.json',
    'kill 123',
    'otel-desktop-viewer',
    'otel-desktop-viewer --open-browser=false',
    'otel-desktop-viewer serve',
    'otel-desktop-viewer start',
    'otel-desktop-viewer stop',
    `echo '${viewer} --help'`,
    `/other/otel-desktop-viewer --help`,
  ])
    assert.equal(
      permissionDenial(event(command), directory, [viewer])?.stopsRun,
      false,
      command
    )
  assert.equal(
    permissionDenial(event('viewer --help', 'read'), directory, [viewer])
      ?.stopsRun,
    false
  )
  assert.equal(
    permissionDenial(event('viewer --help', 'task'), directory, [viewer])
      ?.stopsRun,
    false
  )
})

test('flags before a client subcommand stop without matching flag values or lifecycle calls', () => {
  for (const args of [
    '--endpoint http://127.0.0.1:1 query --help',
    "--endpoint http://127.0.0.1:1 query 'SELECT 1'",
    "--endpoint=http://127.0.0.1:1 query 'SELECT 1'",
    "--limit 2 --endpoint http://127.0.0.1:1 query 'SELECT 1'",
    "--json=true --endpoint http://127.0.0.1:1 query 'SELECT 1'",
    "--json=false --endpoint http://127.0.0.1:1 query 'SELECT 1'",
    "--help=false --endpoint http://127.0.0.1:1 query 'SELECT 1'",
    "-h=false --endpoint http://127.0.0.1:1 query 'SELECT 1'",
    '--service checkout --since 30m --limit 2 logs',
    '--signal logs --owner-type resource attributes keys',
    '--db viewer.duckdb --help',
  ]) {
    const command = `${viewer} ${args}`
    assert.equal(
      permissionDenial(event(command), directory, [viewer])?.stopsRun,
      true,
      command
    )
  }
  for (const command of [
    `${viewer} --endpoint query`,
    `${viewer} --endpoint 'query --help'`,
    `${viewer} --endpoint=query`,
    `${viewer} --endpoint query serve`,
    `${viewer} --endpoint query start`,
    `${viewer} --endpoint query stop`,
    `${viewer} --db query --open-browser=false`,
    `${viewer} --help=false`,
    `${viewer} -h=false`,
    `${viewer} --help=invalid query`,
    `${viewer} --help=true=invalid query`,
    `${viewer} --json=true serve`,
    `${viewer} --json query`,
    `${viewer} --json=invalid query`,
    `${viewer} --unknown query`,
    `${viewer} -- query --help`,
    `${viewer} --endpoint http://127.0.0.1:1 'query --help'`,
    `echo '${viewer} --endpoint http://127.0.0.1:1 query --help'`,
    `printf '%s' '${viewer}' --endpoint query`,
    `/other/otel-desktop-viewer --endpoint http://127.0.0.1:1 query`,
  ]) {
    assert.equal(
      permissionDenial(event(command), directory, [viewer])?.stopsRun,
      false,
      command
    )
  }
})

test('ordinary failures and quoted successful output are not native permission denials', () => {
  for (const line of [
    event(`${viewer} query 'SELECT 1'`, 'bash', 'completed'),
    event(`${viewer} query 'SELECT 1'`, 'bash', 'error', 'command not found'),
    event(
      `${viewer} query 'SELECT 1'`,
      'bash',
      'error',
      'JSON RPC invalid params'
    ),
    event('rg missing', 'bash', 'completed'),
    JSON.stringify({ type: 'text', part: { text: event(`${viewer} --help`) } }),
    event(`${viewer} --help`).replace('"type":"tool"', '"type":"text"'),
    'not json',
  ]) {
    assert.equal(permissionDenial(line, directory, [viewer]), undefined, line)
  }
  const malformed = JSON.parse(event(`${viewer} --help`))
  malformed.part.state.error = reason.replace('"deny"', '"allow"')
  assert.equal(
    permissionDenial(JSON.stringify(malformed), directory, [viewer]),
    undefined
  )
})
