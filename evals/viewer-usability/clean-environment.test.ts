import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import test from 'node:test'
import {
  cleanEnvironment,
  createCleanEnvironment,
} from './clean-environment.ts'
import { record, TEMPORARY } from './runtime.ts'

const parent = () =>
  fs.mkdtempSync(path.join(TEMPORARY, 'eval-isolation-helper-test-'))

test('allowlist scrubs ambient context and keeps only explicitly supplied credentials', () => {
  const inherited = {
    PATH: '/personal/bin',
    HOME: '/personal',
    OPENCODE_CONFIG: '/personal/config',
    OPENCODE_CONFIG_CONTENT: '{"instructions":["answer-key"]}',
    OPENCODE_DB: '/old/session.db',
    OPENCODE_PERMISSION: 'allow',
    OPENCODE_MODELS_PATH: '/answers',
    OPENCODE_TEST_HOME: '/personal',
    OPENCODE_AUTH_CONTENT: '{"openai":{"type":"wellknown","token":"ambient"}}',
    OPENCODE_TEST_MANAGED_CONFIG_DIR: '/personal/managed',
    OPENCODE_CONFIG_DIR: '/personal/config',
    NODE_OPTIONS: '--require /personal/plugin',
    BASH_ENV: '/personal/startup',
    ENV: '/personal/startup',
    CLAUDE_CONFIG_DIR: '/personal',
    ANTHROPIC_API_KEY: 'unselected',
    OPENAI_API_KEY: 'unselected',
    OTEL_EXPORTER_OTLP_HEADERS: 'private',
    HTTPS_PROXY: 'http://proxy.example',
  }
  const env = cleanEnvironment({
    root: '/fresh',
    inherited,
    executablePaths: ['/supplied/bin'],
    authEnvironment: { OPENAI_API_KEY: 'selected-test-value' },
  })
  assert.equal(env.HTTPS_PROXY, inherited.HTTPS_PROXY)
  assert.equal(env.OPENAI_API_KEY, 'selected-test-value')
  for (const key of [
    'OPENCODE_CONFIG',
    'OPENCODE_CONFIG_CONTENT',
    'OPENCODE_DB',
    'OPENCODE_PERMISSION',
    'OPENCODE_MODELS_PATH',
    'OPENCODE_AUTH_CONTENT',
    'NODE_OPTIONS',
    'BASH_ENV',
    'ENV',
    'CLAUDE_CONFIG_DIR',
    'ANTHROPIC_API_KEY',
    'OTEL_EXPORTER_OTLP_HEADERS',
  ])
    assert.equal(env[key], undefined, key)
  assert.equal(env.HOME, '/fresh/home')
  assert.equal(env.OPENCODE_TEST_HOME, env.HOME)
  assert.equal(env.OPENCODE_TEST_MANAGED_CONFIG_DIR, '/fresh/managed')
  assert.equal(env.OPENCODE_CONFIG_DIR, '/fresh/config/opencode')
  assert.equal(env.PATH.split(path.delimiter)[0], '/supplied/bin')
  assert.ok(!env.PATH.includes('/personal'))
  assert.equal(env.OPENCODE_PURE, '1')
  assert.equal(env.OPENCODE_DISABLE_DEFAULT_PLUGINS, undefined)
})

test('fresh roots do not reuse scratch, database or evidence; credentials are selective and private', () => {
  const directory = parent()
  const auth = path.join(directory, 'source-auth.json')
  fs.writeFileSync(
    auth,
    JSON.stringify({
      openai: { type: 'oauth', access: 'synthetic', refresh: 'synthetic' },
      other: { type: 'api', key: 'not-selected' },
    }),
    { mode: 0o600 }
  )
  const original = fs.readFileSync(auth)
  const options = {
    parent: directory,
    model: 'openai/gpt-6.1-sol',
    authFile: auth,
    permission: { bash: 'ask' },
  }
  const first = createCleanEnvironment(options)
  fs.writeFileSync(path.join(first.workspace, 'previous-answer'), 'old')
  fs.writeFileSync(
    path.join(first.env.XDG_DATA_HOME, 'opencode', 'opencode.db'),
    'old'
  )
  const second = createCleanEnvironment(options)
  assert.notEqual(first.root, second.root)
  assert.deepEqual(fs.readdirSync(second.workspace), [])
  assert.ok(
    !fs.existsSync(
      path.join(second.env.XDG_DATA_HOME, 'opencode', 'opencode.db')
    )
  )
  const copy = path.join(second.env.XDG_DATA_HOME, 'opencode', 'auth.json')
  assert.deepEqual(Object.keys(JSON.parse(fs.readFileSync(copy, 'utf8'))), [
    'openai',
  ])
  assert.equal(fs.statSync(copy).mode & 0o777, 0o600)
  assert.equal(fs.statSync(second.root).mode & 0o777, 0o700)
  assert.deepEqual(fs.readFileSync(auth), original)
  for (const key of ['agent', 'instructions', 'plugin', 'mcp', 'references'])
    assert.equal(record(second.config)[key], undefined)
  assert.deepEqual(
    fs.readdirSync(path.join(second.env.OPENCODE_CONFIG_DIR, 'node_modules')),
    []
  )
})

test('missing permissions, remote auth and override credentials fail closed', () => {
  assert.throws(
    () => createCleanEnvironment({ permission: { bash: 'ask' } }),
    /explicit/
  )
  for (const permission of [undefined, {}, 'allow', []]) {
    assert.throws(
      () => createCleanEnvironment({ model: 'openai/gpt-6.1-sol', permission }),
      /permission/
    )
  }
  assert.throws(
    () =>
      cleanEnvironment({
        root: '/fresh',
        authEnvironment: { OPENCODE_CONSOLE_TOKEN: 'x' },
      }),
    /credential/
  )
  const directory = parent()
  const auth = path.join(directory, 'source-auth.json')
  fs.writeFileSync(
    auth,
    JSON.stringify({ openai: { type: 'wellknown', token: 'synthetic' } }),
    { mode: 0o600 }
  )
  assert.throws(
    () =>
      createCleanEnvironment({
        parent: directory,
        model: 'openai/gpt-6.1-sol',
        authFile: auth,
        permission: { bash: 'ask' },
      }),
    /Remote-config/
  )
  assert.throws(
    () =>
      createCleanEnvironment({
        parent: directory,
        model: 'openai/gpt-6.1-sol',
        permission: { bash: 'ask' },
        executablePaths: ['relative'],
      }),
    /absolute/
  )
})

test('scratch inside a repository is rejected without changing that repository', () => {
  const directory = parent()
  fs.writeFileSync(path.join(directory, '.git'), 'synthetic worktree marker')
  assert.throws(
    () =>
      createCleanEnvironment({
        parent: directory,
        model: 'openai/gpt-5.6-sol',
        permission: { bash: 'ask' },
      }),
    /outside repositories/
  )
  assert.deepEqual(fs.readdirSync(directory), ['.git'])
})
