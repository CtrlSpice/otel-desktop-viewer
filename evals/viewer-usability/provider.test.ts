import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { spawnSync } from 'node:child_process'
import Provider from './provider.ts'
import { providerFixture as setup } from './test-fixture.ts'
import { readJson, record } from './runtime.ts'
import { credentialEnvironment } from './clean-environment.ts'

test('timed-out sessions retain partial evidence, terminate their process and allow the next call', async () => {
  const previous = { ...process.env }
  try {
    const { root, isolation } = setup()
    const file = path.join(root, 'stub-settings.json')
    fs.writeFileSync(file, '{"hang":true}')
    const provider = new Provider({
      config: { model: 'openai/gpt-6.1-sol', isolation, timeoutMs: 1000 },
    })
    const timed = await provider.callApi('Synthetic timeout check.', {
      vars: { task_id: 'timeout' },
    })
    assert.match(timed.error || '', /timed out/)
    assert.equal(timed.metadata.timedOut, true)
    assert.ok('exit' in timed.metadata)
    assert.ok(timed.metadata.exit)
    assert.equal(timed.metadata.exit.timedOut, true)
    const pid = Number(
      fs.readFileSync(path.join(timed.metadata.evidence, 'process.pid'), 'utf8')
    )
    assert.throws(() => process.kill(pid, 0), /ESRCH/)
    assert.match(
      fs.readFileSync(
        path.join(timed.metadata.evidence, 'stdout.jsonl'),
        'utf8'
      ),
      /retained partial output/
    )
    assert.equal(
      fs.readFileSync(path.join(timed.metadata.evidence, 'stderr.log'), 'utf8'),
      '[REDACTED]'
    )
    assert.ok(fs.existsSync(path.join(timed.metadata.evidence, 'failure.json')))
    fs.writeFileSync(file, '{}')
    const next = await provider.callApi('Synthetic next task.', {
      vars: { task_id: 'next' },
    })
    assert.equal(next.output, 'test answer [REDACTED]')
    assert.equal(next.metadata.timedOut, false)
    assert.notEqual(next.metadata.evidence, timed.metadata.evidence)
  } finally {
    process.env = previous
  }
})

test('adapter uses actual process boundaries, fresh roots, exact launch and safe evidence', async () => {
  const previous = { ...process.env }
  try {
    const { root, authFile, isolation } = setup()
    process.env.OPENCODE_CONFIG_CONTENT = 'PRIVATE_CONTEXT_CANARY'
    process.env.OPENAI_API_KEY = 'ambient-secret'
    process.env.BASH_ENV = '/personal/startup'
    const original = fs.readFileSync(authFile)
    const prior = path.join(root, 'historical-evidence')
    fs.writeFileSync(prior, 'preserve me')
    const provider = new Provider({
      config: { model: 'openai/gpt-6.1-sol', isolation },
    })
    const first = await provider.callApi(
      'Synthetic adapter test, no evaluation task.',
      { vars: { task_id: 'synthetic' } }
    )
    const retained = fs.readFileSync(
      path.join(first.metadata.evidence, 'launch.json')
    )
    const second = await provider.callApi(
      'Synthetic adapter test, no evaluation task.',
      { vars: { task_id: 'synthetic' } }
    )
    assert.ok(first.tokenUsage)
    assert.ok(second.tokenUsage)
    assert.equal(first.output, 'test answer [REDACTED]')
    assert.equal(second.output, first.output)
    assert.notEqual(first.metadata.workspace, second.metadata.workspace)
    assert.notEqual(first.metadata.evidence, second.metadata.evidence)
    assert.equal(first.metadata.version, '1.18.10')
    assert.equal(first.tokenUsage.prompt, 3)
    assert.equal(first.tokenUsage.completion, 2)
    assert.ok(first.metadata.sessionIDs.length)
    assert.notDeepEqual(first.metadata.sessionIDs, second.metadata.sessionIDs)
    for (const result of [first, second]) {
      assert.ok(result.metadata.workspace)
      const observation = record(
        readJson(path.join(result.metadata.workspace, 'stub-observation.json'))
      )
      const args = observation.args
      assert.ok(
        Array.isArray(args) && args.every(arg => typeof arg === 'string')
      )
      const env = credentialEnvironment(observation.env)
      assert.equal(observation.cwd, result.metadata.workspace)
      assert.equal(args[args.indexOf('--dir') + 1], observation.cwd)
      assert.equal(args[args.indexOf('--model') + 1], 'openai/gpt-6.1-sol')
      assert.equal(args[args.indexOf('--agent') + 1], 'build')
      assert.ok(
        !args.some(arg =>
          ['--continue', '--session', '--attach', '--fork'].includes(arg)
        )
      )
      assert.ok(observation.emptyScratch)
      assert.deepEqual(
        record(observation.config).permission,
        isolation.permission
      )
      for (const key of [
        'OPENCODE_CONFIG_CONTENT',
        'OPENAI_API_KEY',
        'BASH_ENV',
        'OTEL_EVAL_RUN_DIR',
      ])
        assert.equal(env[key], undefined)
      for (const key of [
        'HOME',
        'XDG_CONFIG_HOME',
        'XDG_DATA_HOME',
        'XDG_STATE_HOME',
        'XDG_CACHE_HOME',
        'TMPDIR',
      ]) {
        assert.ok(env[key].startsWith(result.metadata.isolationRoot + path.sep))
      }
      assert.equal(
        spawnSync('sh', ['-c', 'command -v node && command -v bash'], {
          cwd: result.metadata.workspace,
          env,
        }).status,
        0
      )
      for (const name of fs.readdirSync(result.metadata.evidence)) {
        const file = path.join(result.metadata.evidence, name)
        const content = fs.readFileSync(file, 'utf8')
        for (const marker of [
          'synthetic-selected-secret',
          'unselected-secret',
          'ambient-secret',
          'PRIVATE_CONTEXT_CANARY',
        ])
          assert.ok(!content.includes(marker), name)
        assert.equal(fs.statSync(file).mode & 0o777, 0o600)
      }
      assert.equal(fs.statSync(result.metadata.evidence).mode & 0o777, 0o700)
      assert.equal(
        fs.readFileSync(
          path.join(result.metadata.evidence, 'stderr.log'),
          'utf8'
        ),
        '[REDACTED]'
      )
    }
    assert.deepEqual(fs.readFileSync(authFile), original)
    assert.equal(fs.statSync(authFile).mode & 0o777, 0o600)
    assert.deepEqual(
      fs.readFileSync(path.join(first.metadata.evidence, 'launch.json')),
      retained
    )
    assert.equal(fs.readFileSync(prior, 'utf8'), 'preserve me')
  } finally {
    process.env = previous
  }
})

test('missing policy/model, unsupported version, spawn and incomplete sessions retain failures', async () => {
  const previous = { ...process.env }
  try {
    const { root, isolation } = setup()
    const settings = path.join(root, 'stub-settings.json')
    const call = async (config: { isolation?: unknown }) =>
      new Provider({
        config: { model: 'openai/gpt-6.1-sol', ...config },
      }).callApi('Synthetic failure check.', { vars: { task_id: 'failure' } })
    const missing = await call({})
    assert.match(missing.error || '', /Caller must supply/)
    fs.chmodSync(isolation.authFile, 0o644)
    const publicAuth = await call({ isolation })
    assert.match(publicAuth.error || '', /private absolute credential/)
    fs.chmodSync(isolation.authFile, 0o600)
    const inlineAuth = await call({
      isolation: {
        ...isolation,
        authEnvironment: { OPENAI_API_KEY: 'inline-synthetic-secret' },
      },
    })
    assert.match(inlineAuth.error || '', /not inline credentials/)
    assert.ok(
      !fs
        .readFileSync(
          path.join(inlineAuth.metadata.evidence, 'failure.json'),
          'utf8'
        )
        .includes('inline-synthetic-secret')
    )
    fs.writeFileSync(settings, JSON.stringify({ version: '99.0.0' }))
    const version = await call({ isolation })
    assert.match(version.error || '', /Unsupported/)
    assert.ok(
      !fs.existsSync(path.join(version.metadata.evidence, 'models.stdout'))
    )
    fs.writeFileSync(settings, JSON.stringify({ models: 'openai/gpt-5.6-sol' }))
    const model = await call({ isolation })
    assert.match(model.error || '', /absent/)
    assert.ok(!fs.existsSync(path.join(model.metadata.evidence, 'launch.json')))
    assert.ok(
      fs.existsSync(
        path.join(model.metadata.evidence, 'models-refreshed.stdout')
      )
    )
    fs.writeFileSync(
      settings,
      JSON.stringify({
        models: 'openai/gpt-5.6-sol',
        refreshedModels: 'openai/gpt-6.1-sol',
      })
    )
    const refreshed = await call({ isolation })
    assert.equal(refreshed.output, 'test answer [REDACTED]')
    const rejectedPrompt = await new Provider({
      config: { model: 'openai/gpt-6.1-sol', isolation },
    }).callApi('synthetic-selected-secret', {
      vars: { task_id: 'credential-prompt' },
    })
    assert.match(rejectedPrompt.error || '', /credential material/)
    assert.ok(
      !fs.existsSync(path.join(rejectedPrompt.metadata.evidence, 'prompt.txt'))
    )
    const spawn = await call({
      isolation: { ...isolation, opencodeBinary: path.join(root, 'missing') },
    })
    assert.match(spawn.error || '', /spawn failed: ENOENT/)
    fs.writeFileSync(settings, JSON.stringify({ incomplete: true }))
    const incomplete = await call({ isolation })
    assert.match(incomplete.error || '', /Incomplete/)
    assert.ok('exit' in incomplete.metadata)
    assert.ok(incomplete.metadata.exit)
    assert.equal(incomplete.metadata.exit.code, 1)
    assert.ok(
      fs.existsSync(path.join(incomplete.metadata.evidence, 'stdout.jsonl'))
    )
    for (const result of [missing, version, model, spawn])
      assert.ok(
        fs.existsSync(path.join(result.metadata.evidence, 'failure.json'))
      )
  } finally {
    process.env = previous
  }
})

test('selected private environment credentials and tool metrics survive the process boundary safely', async () => {
  const previous = { ...process.env }
  try {
    const { root, isolation } = setup()
    const file = path.join(root, 'environment.json')
    fs.writeFileSync(
      file,
      '{"SYNTHETIC_API_KEY":"synthetic-environment-secret"}',
      { mode: 0o600 }
    )
    fs.writeFileSync(path.join(root, 'stub-settings.json'), '{"tools":true}')
    const provider = new Provider({
      config: {
        model: 'openai/gpt-6.1-sol',
        isolation: { ...isolation, authEnvironmentFile: file },
      },
    })
    const result = await provider.callApi('Synthetic tool metrics.', {
      vars: { task_id: 'metrics' },
    })
    assert.ok(result.tokenUsage)
    assert.equal(result.output, 'test answer [REDACTED]')
    assert.equal(result.metadata.toolCalls, 2)
    assert.equal(result.metadata.utilityCommandEntries, 1)
    assert.equal(result.metadata.queryCommandEntries, 1)
    assert.equal(result.metadata.failedToolCalls, 1)
    assert.equal(
      fs.readFileSync(
        path.join(result.metadata.evidence, 'stderr.log'),
        'utf8'
      ),
      '[REDACTED][REDACTED]'
    )
    assert.equal(
      fs.readFileSync(file, 'utf8'),
      '{"SYNTHETIC_API_KEY":"synthetic-environment-secret"}'
    )
    fs.writeFileSync(
      path.join(root, 'stub-settings.json'),
      '{"badTokens":true}'
    )
    const malformed = await provider.callApi('Synthetic malformed protocol.', {
      vars: { task_id: 'malformed' },
    })
    assert.match(malformed.error || '', /session processing failed/)
    assert.ok(
      fs.existsSync(path.join(malformed.metadata.evidence, 'stdout.jsonl'))
    )
    for (const name of fs.readdirSync(malformed.metadata.evidence)) {
      const text = fs.readFileSync(
        path.join(malformed.metadata.evidence, name),
        'utf8'
      )
      assert.ok(!text.includes('synthetic-selected-secret'))
      assert.ok(!text.includes('synthetic-environment-secret'))
    }
  } finally {
    process.env = previous
  }
})
