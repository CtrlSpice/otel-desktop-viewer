import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import http from 'node:http'
import { once } from 'node:events'
import { spawn, spawnSync } from 'node:child_process'
import { createCleanEnvironment } from './clean-environment.ts'
import { TEMPORARY } from './runtime.ts'
import { freePorts, query } from './prepare.ts'
import Provider from './provider.ts'
import { permissionDenial } from './permission-stop.ts'

test('native 1.18.10 allows scratch executable workdirs and preserves denials', async () => {
  const parent = fs.mkdtempSync(
    path.join(TEMPORARY, 'eval-native-permissions-')
  )
  const permission = {
    '*': 'allow',
    external_directory: 'deny',
    task: 'deny',
    read: { '*': 'allow', '*/auth.json': 'deny', '*/opencode.db*': 'deny' },
    bash: {
      '*': 'allow',
      '*otel-desktop-viewer': 'deny',
      '*otel-desktop-viewer --*': 'deny',
      '*otel-desktop-viewer --help': 'allow',
      '*otel-desktop-viewer -h': 'allow',
      '*otel-desktop-viewer serve*': 'deny',
      '*kill *': 'deny',
    },
  }
  const clean = createCleanEnvironment({
    parent,
    model: 'canary/test',
    permission,
  })
  const version = spawnSync('opencode', ['--version'], {
    cwd: clean.workspace,
    env: clean.env,
    encoding: 'utf8',
    timeout: 30000,
  })
  assert.equal(version.stdout.trim(), '1.18.10')
  const directory = path.join(clean.workspace, 'bin')
  fs.mkdirSync(directory)
  const viewer = path.join(directory, 'otel-desktop-viewer')
  const source = process.env.OTEL_EVAL_VIEWER_BINARY
  assert.ok(
    source && path.isAbsolute(source),
    'Set absolute OTEL_EVAL_VIEWER_BINARY to the verification binary'
  )
  fs.copyFileSync(source, viewer, fs.constants.COPYFILE_EXCL)
  fs.chmodSync(viewer, 0o700)
  const forbidden = path.join(parent, 'other-session')
  fs.mkdirSync(forbidden)
  fs.writeFileSync(
    path.join(forbidden, 'rubric.txt'),
    'private synthetic rubric'
  )
  fs.writeFileSync(path.join(forbidden, 'source.ts'), 'synthetic source marker')
  fs.writeFileSync(
    path.join(forbidden, 'fixture.json'),
    'synthetic fixture marker'
  )
  fs.writeFileSync(
    path.join(clean.workspace, 'auth.json'),
    'synthetic credential marker'
  )
  fs.writeFileSync(
    path.join(clean.workspace, 'opencode.db'),
    'synthetic session marker'
  )
  fs.writeFileSync(
    path.join(clean.workspace, 'no-matches.txt'),
    'synthetic marker\n'
  )
  let input: { tool: string; args: unknown }
  const server = http.createServer(async (req, res) => {
    let body = ''
    for await (const chunk of req) body += chunk
    const request = JSON.parse(body)
    const tool =
      request.tools?.length &&
      !request.messages?.some((item: { role: string }) => item.role === 'tool')
    const delta = tool
      ? {
          role: 'assistant',
          tool_calls: [
            {
              index: 0,
              id: 'call_canary',
              type: 'function',
              function: {
                name: input.tool,
                arguments: JSON.stringify(input.args),
              },
            },
          ],
        }
      : { role: 'assistant', content: 'synthetic complete' }
    res.writeHead(200, { 'Content-Type': 'text/event-stream' })
    res.end(
      'data: ' +
        JSON.stringify({
          id: 'canary',
          object: 'chat.completion.chunk',
          created: 0,
          model: 'test',
          choices: [{ index: 0, delta, finish_reason: null }],
        }) +
        '\n\ndata: ' +
        JSON.stringify({
          id: 'canary',
          object: 'chat.completion.chunk',
          created: 0,
          model: 'test',
          choices: [
            {
              index: 0,
              delta: {},
              finish_reason: tool ? 'tool_calls' : 'stop',
            },
          ],
        }) +
        '\n\ndata: [DONE]\n\n'
    )
  })
  server.listen(0, '127.0.0.1')
  await once(server, 'listening')
  const address = server.address()
  assert.ok(address && typeof address !== 'string')
  fs.writeFileSync(
    path.join(clean.env.OPENCODE_CONFIG_DIR, 'opencode.json'),
    JSON.stringify({
      ...clean.config,
      provider: {
        canary: {
          npm: '@ai-sdk/openai-compatible',
          options: {
            baseURL: `http://127.0.0.1:${address.port}/v1`,
            apiKey: 'synthetic-only',
          },
          models: {
            test: {
              name: 'Synthetic Test',
              tool_call: true,
              limit: { context: 128000, output: 1000 },
            },
          },
        },
      },
    })
  )
  const [ui, grpc, otlp] = await freePorts()
  const endpoint = `http://127.0.0.1:${ui}`
  const owned = spawn(
    viewer,
    [
      '--open-browser=false',
      '--host',
      '127.0.0.1',
      '--browser-port',
      String(ui),
      '--grpc',
      String(grpc),
      '--http',
      String(otlp),
      '--db',
      path.join(parent, 'verification.duckdb'),
      '--db-max-size',
      '0',
    ],
    { cwd: parent, stdio: ['ignore', 'pipe', 'pipe'] }
  )
  let viewerOutput = ''
  owned.stdout.on('data', data => {
    viewerOutput += data
  })
  owned.stderr.on('data', data => {
    viewerOutput += data
  })
  const closed = new Promise(resolve =>
    owned.once('close', (code, signal) => resolve({ code, signal }))
  )
  try {
    let ready = false
    for (let attempt = 0; attempt < 100; attempt++) {
      assert.equal(owned.exitCode, null, viewerOutput)
      try {
        await query(endpoint, 'SELECT 1')
        ready = true
        break
      } catch {}
      await new Promise(resolve => setTimeout(resolve, 100))
    }
    assert.ok(ready, 'Owned viewer did not become ready: ' + viewerOutput)
    const models = spawnSync('opencode', ['models', 'canary'], {
      cwd: clean.workspace,
      env: clean.env,
      encoding: 'utf8',
      timeout: 30000,
    })
    assert.equal(models.status, 0, models.stderr)
    const agent = spawnSync('opencode', ['debug', 'agent', 'build'], {
      cwd: clean.workspace,
      env: clean.env,
      encoding: 'utf8',
      timeout: 30000,
    })
    assert.equal(agent.status, 0, agent.stderr)
    assert.equal(
      JSON.parse(agent.stdout).tools.task,
      false,
      'Delegation stays disabled'
    )
    for (const [name, tool, args, denied, expectedExit = 0] of [
      [
        'absolute-help',
        'bash',
        { command: viewer + ' --help', workdir: clean.workspace },
        false,
      ],
      [
        'relative-help',
        'bash',
        { command: './otel-desktop-viewer --help', workdir: directory },
        false,
      ],
      [
        'absolute-query',
        'bash',
        {
          command: viewer + ' query "SELECT 1" --endpoint ' + endpoint,
          workdir: clean.workspace,
        },
        false,
      ],
      [
        'relative-query',
        'bash',
        {
          command:
            './otel-desktop-viewer query "SELECT 1" --endpoint ' + endpoint,
          workdir: directory,
        },
        false,
      ],
      [
        'command-not-found',
        'bash',
        {
          command: 'eval_synthetic_command_does_not_exist',
          workdir: clean.workspace,
        },
        false,
        127,
      ],
      [
        'empty-search',
        'bash',
        { command: 'rg "^absent$" no-matches.txt', workdir: clean.workspace },
        false,
        1,
      ],
      [
        'query-error',
        'bash',
        {
          command:
            viewer +
            ' query "SELECT nonexistent_column" --endpoint ' +
            endpoint,
          workdir: clean.workspace,
        },
        false,
        1,
      ],
      [
        'rubric',
        'read',
        { filePath: path.join(forbidden, 'rubric.txt') },
        true,
      ],
      [
        'credential',
        'read',
        { filePath: path.join(clean.workspace, 'auth.json') },
        true,
      ],
      ['source', 'read', { filePath: path.join(forbidden, 'source.ts') }, true],
      [
        'fixture',
        'read',
        { filePath: path.join(forbidden, 'fixture.json') },
        true,
      ],
      [
        'session-history',
        'read',
        { filePath: path.join(clean.workspace, 'opencode.db') },
        true,
      ],
      [
        'lifecycle',
        'bash',
        { command: viewer + ' serve', workdir: directory },
        true,
      ],
    ] as const) {
      input = { tool, args }
      const child = spawn(
        'opencode',
        [
          'run',
          '--dir',
          clean.workspace,
          '--agent',
          'build',
          '--model',
          'canary/test',
          '--format',
          'json',
          '--auto',
          'Synthetic native permission check.',
        ],
        {
          cwd: clean.workspace,
          env: clean.env,
          stdio: ['ignore', 'pipe', 'pipe'],
        }
      )
      let stdout = '',
        stderr = ''
      child.stdout.on('data', data => {
        stdout += data
      })
      child.stderr.on('data', data => {
        stderr += data
      })
      const timer = setTimeout(() => child.kill('SIGKILL'), 60000)
      const [code] = await once(child, 'close')
      clearTimeout(timer)
      fs.writeFileSync(path.join(parent, name + '.jsonl'), stdout, {
        mode: 0o600,
      })
      fs.writeFileSync(path.join(parent, name + '.stderr'), stderr, {
        mode: 0o600,
      })
      assert.equal(code, 0, name + ': ' + stderr)
      const event = stdout
        .split('\n')
        .filter(Boolean)
        .map(line => JSON.parse(line))
        .find(event => event.type === 'tool_use')
      assert.ok(event, name + ': no tool event; evidence ' + parent)
      assert.equal(
        event.part.state.status,
        denied ? 'error' : 'completed',
        name
      )
      if (denied)
        assert.match(
          event.part.state.error,
          /^The user has specified a rule which prevents you from using this specific tool call\./
        )
      else assert.equal(event.part.state.metadata.exit, expectedExit, name)
      assert.equal(
        permissionDenial(JSON.stringify(event), clean.workspace, [viewer])
          ?.stopsRun,
        denied ? false : undefined,
        name
      )
    }
    const previous = process.env.OTEL_EVAL_RUN_DIR
    const rootViewer = path.join(parent, 'otel-desktop-viewer')
    fs.copyFileSync(source, rootViewer, fs.constants.COPYFILE_EXCL)
    const wrapper = path.join(parent, 'native-wrapper.cjs')
    fs.writeFileSync(
      wrapper,
      `#!${process.execPath}
const fs=require('node:fs'),path=require('node:path'),{spawn}=require('node:child_process');
const file=path.join(process.env.OPENCODE_CONFIG_DIR,'opencode.json');
const config=JSON.parse(fs.readFileSync(file));
config.provider=${JSON.stringify({ canary: { npm: '@ai-sdk/openai-compatible', options: { baseURL: `http://127.0.0.1:${address.port}/v1`, apiKey: 'synthetic-only' }, models: { test: { name: 'Synthetic Test', tool_call: true, limit: { context: 128000, output: 1000 } } } } })};
fs.writeFileSync(file,JSON.stringify(config));
const child=spawn('opencode',process.argv.slice(2),{stdio:'inherit'});
child.once('close',code=>{process.exitCode=code??1;});
`,
      { mode: 0o700 }
    )
    try {
      process.env.OTEL_EVAL_RUN_DIR = parent
      input = {
        tool: 'bash',
        args: { command: './otel-desktop-viewer --help', workdir: parent },
      }
      const result = await new Provider({
        config: {
          model: 'canary/test',
          isolation: { permission, opencodeBinary: wrapper },
        },
      }).callApi('Viewer executable: ' + rootViewer, {
        vars: { task_id: 'native-blocked-help' },
      })
      assert.match(result.error ?? '', /Harness blocker/)
      assert.equal(result.metadata.timedOut, false)
      assert.equal(
        fs.readdirSync(
          path.join(
            result.metadata.isolationRoot!,
            'config/opencode/node_modules'
          )
        ).length,
        0
      )
      const blocker = JSON.parse(
        fs.readFileSync(path.join(parent, 'harness-blocker.json'), 'utf8')
      )
      assert.equal(blocker.denial.stopsRun, true)
      assert.equal(blocker.denial.input.command, './otel-desktop-viewer --help')
      assert.equal(blocker.denial.input.workdir, parent)
      assert.throws(() => process.kill(blocker.sessionPID, 0), /ESRCH/)
      const retained = fs.readFileSync(
        path.join(result.metadata.evidence, 'stdout.jsonl'),
        'utf8'
      )
      assert.ok(retained.includes(blocker.denial.rawEvent))
      fs.unlinkSync(
        path.join(result.metadata.workspace!, 'bin/otel-desktop-viewer')
      )
    } finally {
      if (previous === undefined) delete process.env.OTEL_EVAL_RUN_DIR
      else process.env.OTEL_EVAL_RUN_DIR = previous
      fs.unlinkSync(rootViewer)
    }
    assert.equal(
      fs.readdirSync(path.join(clean.env.OPENCODE_CONFIG_DIR, 'node_modules'))
        .length,
      0,
      'Native check must not install dependencies'
    )
    console.log('Native synthetic permission evidence: ' + parent)
  } finally {
    owned.kill('SIGTERM')
    const timer = setTimeout(() => owned.kill('SIGKILL'), 30000)
    try {
      const exit = await closed
      fs.writeFileSync(
        path.join(parent, 'owned-shutdown.json'),
        JSON.stringify({ pid: owned.pid, waited: true, exit }),
        { mode: 0o600 }
      )
      fs.writeFileSync(path.join(parent, 'viewer.log'), viewerOutput, {
        mode: 0o600,
      })
    } finally {
      clearTimeout(timer)
    }
    await new Promise<void>(resolve => server.close(() => resolve()))
    fs.unlinkSync(viewer)
  }
})
