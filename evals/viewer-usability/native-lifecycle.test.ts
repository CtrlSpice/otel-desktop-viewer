import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import http from 'node:http'
import { once } from 'node:events'
import { TEMPORARY, SUITE } from './runtime.ts'
import { stageDataset, datasetSource, requestList, readManifest } from './stage-dataset.ts'
import { scratchFiles, verifyExport } from './verify-export.ts'

test('native hooks copy fresh workspaces, retain complete evidence and clean owned runtimes', { timeout: 240000 }, async () => {
  const root = fs.mkdtempSync(path.join(TEMPORARY, 'eval-native-hooks-test-'))
  fs.chmodSync(root, 0o700)
  fs.mkdirSync(path.join(root, 'tmp'), { mode: 0o700 })
  stageDataset(datasetSource(), path.join(root, 'fixture'))
  const traceFile = requestList(readManifest(path.join(root, 'fixture/manifest.json'))).find(request => request.signal === 'traces')!.file
  const traceBytes = fs.readFileSync(path.join(root, 'fixture', traceFile))
  const requests: string[] = []
  const service = http.createServer(async (req, res) => {
    let body = ''
    for await (const chunk of req) body += chunk
    requests.push(body)
    if (body.includes('SYNTHETIC_TIMEOUT')) return
    const request = JSON.parse(body)
    const tools = (request.messages ?? []).filter((m: { role: string }) => m.role === 'tool')
    const step = tools.length
    let call: { name: string; arguments: string } | undefined
    if (step === 0) call = { name: 'bash', arguments: JSON.stringify({ command: 'cd bin && ./otel-desktop-viewer > help.txt --help && test ! -e artifact.txt && printf "9007199254740993" > ../artifact.txt && node -e \'console.log("NATIVE_TMP="+process.env.TMPDIR);console.log("NATIVE_DATA="+process.env.XDG_DATA_HOME)\'' }) }
    const temp = tools[0]?.content?.match(/NATIVE_TMP=([^\n]+)/)?.[1]
    const data = tools[0]?.content?.match(/NATIVE_DATA=([^\n]+)/)?.[1]
    if (step === 1 && temp) {
      const directory = path.join(temp, 'opencode/checkout-017')
      call = { name: 'bash', arguments: JSON.stringify({ command: `mkdir -p "${directory}" && node -e 'require("node:fs").writeFileSync(${JSON.stringify(path.join(directory, 'handoff.json'))},Buffer.from("${traceBytes.toString('base64')}","base64"))'` }) }
    }
    if (step === 2) call = { name: 'read', arguments: JSON.stringify({ filePath: path.join(root, 'auth-env.json') }) }
    if (step === 3) call = { name: 'bash', arguments: JSON.stringify({ command: `mkdir -p "${root}/forbidden-export"` }) }
    if (step === 4 && data) call = { name: 'read', arguments: JSON.stringify({ filePath: path.join(data, 'opencode/auth.json') }) }
    if (step === 5) call = { name: 'bash', arguments: JSON.stringify({ command: `mkdir -p "${root}/workspaces/sibling-export"` }) }
    if (step === 6) call = { name: 'bash', arguments: JSON.stringify({ command: `node -e 'console.log("x".repeat(70000)+"\\nNATIVE_FULL_OUTPUT_MARKER 9007199254740993")'` }) }
    if (step === 7) {
      const saved = JSON.stringify(tools.at(-1)).match(/\/[\w/.-]+\/tool-output\/tool_[A-Za-z0-9]+/)?.[0]
      if (saved) call = { name: 'grep', arguments: JSON.stringify({ pattern: 'NATIVE_FULL_OUTPUT_MARKER', path: path.dirname(saved), include: path.basename(saved) }) }
    }
    const passed = step >= 8 && JSON.stringify(tools.at(-1)).includes('Line 2: NATIVE_FULL_OUTPUT_MARKER 9007199254740993')
    const delta = call ? { role: 'assistant', tool_calls: [{ index: 0, id: 'native_check_' + step, type: 'function', function: call }] } : { role: 'assistant', content: passed ? 'native-pass' : 'native-failed' }
    const event = (delta: unknown, finish_reason: string | null) => 'data: ' + JSON.stringify({ id: 'synthetic', object: 'chat.completion.chunk', created: 0, model: 'test', choices: [{ index: 0, delta, finish_reason }], usage: { prompt_tokens: 10, completion_tokens: 2, total_tokens: 12 } }) + '\n\n'
    res.writeHead(200, { 'Content-Type': 'text/event-stream' })
    res.end(event(delta, null) + event({}, call ? 'tool_calls' : 'stop') + 'data: [DONE]\n\n')
  })
  service.listen(0, '127.0.0.1')
  await once(service, 'listening')
  const address = service.address()
  assert.ok(address && typeof address !== 'string')
  const binary = path.join(root, 'synthetic-opencode')
  fs.writeFileSync(binary, `#!${process.execPath}
import fs from 'node:fs';
import {spawn} from 'node:child_process';
if(process.argv[2]==='--version') console.log('1.18.10');
else if(process.argv[2]==='models') console.log('canary/test');
else {
 const file=process.env.OPENCODE_CONFIG_DIR+'/opencode.json';
 const config=JSON.parse(fs.readFileSync(file,'utf8'));
 config.provider={canary:{npm:'@ai-sdk/openai-compatible',options:{baseURL:'http://127.0.0.1:${address.port}/v1',apiKey:'synthetic-only'},models:{test:{name:'Synthetic native check',tool_call:true,limit:{context:128000,output:2000}}}}};
 fs.writeFileSync(file,JSON.stringify(config),{mode:0o600});
 const descendant=spawn(process.execPath,['-e',"process.on('SIGTERM',()=>{});process.send('ready');setInterval(()=>{},1000);"],{stdio:['ignore','ignore','ignore','ipc']});
 descendant.once('message',()=>{fs.writeFileSync(process.env.XDG_STATE_HOME+'/descendant.pid',String(descendant.pid),{mode:0o600});descendant.disconnect();});
 const child=spawn('/opt/homebrew/bin/opencode',process.argv.slice(2),{stdio:'inherit'});
 child.on('close',code=>process.exitCode=code??1);
}
`, { mode: 0o700 })
  // A real executable, not inference, provides deterministic help output.
  const viewer = process.env.OTEL_EVAL_VIEWER_BINARY ?? '/usr/bin/true'
  fs.copyFileSync(viewer, path.join(root, 'otel-desktop-viewer'))
  const auth = path.join(root, 'auth-env.json')
  fs.writeFileSync(auth, '{"CANARY_API_KEY":"synthetic-only"}', { mode: 0o600 })
  const isolation = path.join(root, 'isolation.json')
  fs.writeFileSync(isolation, JSON.stringify({ permission: { '*': 'allow', task: 'deny', external_directory: 'deny' }, opencodeBinary: binary, authEnvironmentFile: auth }), { mode: 0o600 })
  const driver = path.join(root, 'driver.mjs')
  fs.writeFileSync(driver, `import fs from 'node:fs';
import {evaluate} from ${JSON.stringify(path.join(SUITE, 'node_modules/promptfoo/dist/src/index.js'))};
const result=await evaluate({prompts:['Synthetic task. Executable: {{binary}}'],providers:['echo'],extensions:${JSON.stringify(['beforeEach', 'afterEach', 'afterAll'].map(hook => 'file://' + path.join(SUITE, 'native-hooks.mjs') + ':' + hook))},tests:[{vars:{eval_model:'canary/test',task_id:'synthetic',binary:'private-run-path'},assert:[{type:'javascript',value:'output === "native-pass"'}]}]}, {cache:false,share:false,writeLatestResults:false,maxConcurrency:1,repeat:2});
fs.writeFileSync(process.env.NATIVE_TEST_RESULT || ${JSON.stringify(path.join(root, 'summary.json'))},JSON.stringify(await result.toEvaluateSummary()),{mode:0o600});
`, { mode: 0o600 })
  try {
    const { cleanEnvironment } = await import('./clean-environment.ts')
    const { spawn } = await import('node:child_process')
    const child = spawn(process.execPath, [driver], { env: { ...cleanEnvironment({ root }), OTEL_EVAL_RUN_DIR: root, OTEL_EVAL_ISOLATION_FILE: isolation, PROMPTFOO_DISABLE_TELEMETRY: '1', PROMPTFOO_CONFIG_DIR: path.join(root, 'promptfoo-state') }, stdio: ['ignore', 'pipe', 'pipe'] })
    let stdout = '', stderr = ''
    child.stdout.on('data', chunk => { stdout += chunk })
    child.stderr.on('data', chunk => { stderr += chunk })
    const timer = setTimeout(() => child.kill('SIGTERM'), 150000)
    const [code] = await once(child, 'close')
    clearTimeout(timer)
    fs.writeFileSync(path.join(root, 'driver.stdout.log'), stdout, { mode: 0o600 })
    fs.writeFileSync(path.join(root, 'driver.stderr.log'), stderr, { mode: 0o600 })
    assert.equal(code, 0, 'Native driver failed; evidence: ' + root)
    const summary = JSON.parse(fs.readFileSync(path.join(root, 'summary.json'), 'utf8'))
    assert.equal(summary.results.length, 2)
    for (const row of summary.results) assert.equal(row.success, true, JSON.stringify(row.response ?? row.error))
    const metrics = fs.readdirSync(path.join(root, 'runs')).map(name => JSON.parse(fs.readFileSync(path.join(root, 'runs', name, 'metrics.json'), 'utf8')))
    assert.equal(new Set(metrics.map(m => m.isolationRoot)).size, 2)
    assert.equal(new Set(metrics.flatMap(m => m.sessionIDs)).size, 2)
    assert.equal(new Set(metrics.map(m => m.workspace)).size, 2)
    const temps = metrics.map(m => JSON.parse(fs.readFileSync(path.join(m.evidence, 'runtime.json'), 'utf8')).tmpdir)
    assert.equal(new Set(temps).size, 2)
    for (const m of metrics) {
      assert.equal(m.toolCalls, 8)
      assert.equal(m.usage.input, 90)
      assert.equal(m.usage.output, 18)
      const runtime = JSON.parse(fs.readFileSync(path.join(m.evidence, 'runtime.json'), 'utf8'))
      assert.equal(runtime.tmpdir, path.join(m.workspace, 'tmp'))
      assert.notEqual(m.workspace, m.archivedWorkspace)
      assert.ok(!runtime.tmpdir.startsWith(m.isolationRoot + path.sep))
      const handoff = path.join(m.archivedWorkspace, 'tmp/opencode/checkout-017/handoff.json')
      assert.deepEqual(fs.readFileSync(handoff), traceBytes)
      assert.ok(scratchFiles(m.archivedWorkspace).includes(handoff))
      if (process.env.OTEL_EVAL_VIEWER_BINARY) {
        fs.writeFileSync(path.join(root, 'connection.json'), JSON.stringify({ binary: viewer }), { mode: 0o600 })
        const verified = await verifyExport({ ...m, workspace: m.archivedWorkspace }, root)
        assert.equal(verified.pass, true, verified.reason)
        assert.ok(verified.files.includes(handoff))
      }
      const denials = fs.readFileSync(path.join(m.evidence, 'permission-denials.jsonl'), 'utf8').trim().split('\n').map(line => JSON.parse(line))
      assert.equal(denials.length, 4)
      assert.ok(denials.every(denial => !denial.stopsRun))
      assert.ok(!fs.existsSync(path.join(root, 'forbidden-export')))
      assert.ok(!fs.existsSync(path.join(root, 'workspaces/sibling-export')))
      const stdout = fs.readFileSync(path.join(m.evidence, 'stdout.jsonl'), 'utf8')
      assert.ok(stdout.includes('9007199254740993'))
      assert.ok(stdout.includes(`mkdir -p \\"${runtime.tmpdir}/opencode/checkout-017\\"`))
      assert.ok(requests.some(body => JSON.stringify(JSON.parse(body).tools).includes(runtime.tmpdir + '/opencode')), 'Bash instructions must name the bound temporary directory')
      assert.equal(fs.readFileSync(path.join(m.archivedWorkspace, 'artifact.txt'), 'utf8'), '9007199254740993')
      assert.ok(fs.existsSync(path.join(m.evidence, 'native-history.json')))
      const pid = Number(fs.readFileSync(path.join(m.evidence, 'process.pid'), 'utf8'))
      assert.throws(() => process.kill(pid, 0))
      const descendant = Number(fs.readFileSync(path.join(m.isolationRoot, 'state/descendant.pid'), 'utf8'))
      assert.throws(() => process.kill(descendant, 0))
    }
    for (const body of requests) for (const forbidden of ['private-run-path', 'rubrics.json', 'SHARED_INSTRUCTIONS', 'native-eval-hooks-worker', 'gradingPrompt']) assert.ok(!body.includes(forbidden), forbidden)
    const requestCount = requests.length
    const deniedIsolation = { permission: { '*': 'allow', bash: { '*': 'allow', '*otel-desktop-viewer*': 'deny' }, task: 'deny', external_directory: { '*': 'deny' } }, opencodeBinary: binary, authEnvironmentFile: auth }
    fs.writeFileSync(isolation, JSON.stringify(deniedIsolation), { mode: 0o600 })
    const denied = spawn(process.execPath, [driver], { env: { ...cleanEnvironment({ root }), OTEL_EVAL_RUN_DIR: root, OTEL_EVAL_ISOLATION_FILE: isolation, NATIVE_TEST_RESULT: path.join(root, 'summary-denial.json'), PROMPTFOO_DISABLE_TELEMETRY: '1', PROMPTFOO_CONFIG_DIR: path.join(root, 'promptfoo-state') }, stdio: ['ignore', 'pipe', 'pipe'] })
    let deniedLog = ''
    denied.stdout.on('data', chunk => { deniedLog += chunk })
    denied.stderr.on('data', chunk => { deniedLog += chunk })
    const deniedTimer = setTimeout(() => denied.kill('SIGTERM'), 75000)
    const [deniedCode] = await once(denied, 'close')
    clearTimeout(deniedTimer)
    fs.writeFileSync(path.join(root, 'denial-driver.log'), deniedLog, { mode: 0o600 })
    assert.equal(deniedCode, 1)
    const blocker = JSON.parse(fs.readFileSync(path.join(root, 'harness-blocker.json'), 'utf8'))
    assert.equal(blocker.denial.stopsRun, true)
    assert.ok(blocker.denial.input.command.startsWith('cd bin'))
    assert.ok(fs.existsSync(path.join(blocker.evidence, 'native-events.jsonl')))
    assert.throws(() => process.kill(blocker.sessionPID, 0))
    assert.ok(requests.length - requestCount <= 2, 'Denial did not stop active session promptly')
    const askRoot = path.join(root, 'ask-run')
    fs.mkdirSync(path.join(askRoot, 'tmp'), { recursive: true, mode: 0o700 })
    fs.copyFileSync(viewer, path.join(askRoot, 'otel-desktop-viewer'))
    fs.writeFileSync(isolation, JSON.stringify({ ...deniedIsolation, permission: { ...deniedIsolation.permission, bash: { '*': 'allow', '*otel-desktop-viewer*': 'ask' } } }), { mode: 0o600 })
    const ask = spawn(process.execPath, [driver], { env: { ...cleanEnvironment({ root: askRoot }), OTEL_EVAL_RUN_DIR: askRoot, OTEL_EVAL_ISOLATION_FILE: isolation, PROMPTFOO_DISABLE_TELEMETRY: '1', PROMPTFOO_CONFIG_DIR: path.join(askRoot, 'promptfoo-state') }, stdio: ['ignore', 'pipe', 'pipe'] })
    let askLog = ''
    ask.stdout.on('data', chunk => { askLog += chunk })
    ask.stderr.on('data', chunk => { askLog += chunk })
    const askTimer = setTimeout(() => ask.kill('SIGTERM'), 75000)
    const [askCode] = await once(ask, 'close')
    clearTimeout(askTimer)
    fs.writeFileSync(path.join(root, 'ask-driver.log'), askLog, { mode: 0o600 })
    assert.equal(askCode, 1, askLog)
    const askBlocker = JSON.parse(fs.readFileSync(path.join(askRoot, 'harness-blocker.json'), 'utf8'))
    assert.equal(askBlocker.denial.stopsRun, true)
    assert.equal(askBlocker.denial.reason, 'Native permission request is pending')
    assert.throws(() => process.kill(askBlocker.sessionPID, 0))
    const deadlineDriver = path.join(root, 'deadline.mjs')
    fs.writeFileSync(deadlineDriver, `import fs from 'node:fs'; import {NativeAttempt} from ${JSON.stringify(path.join(SUITE, 'native-lifecycle.ts'))};
const attempt=await NativeAttempt.create('canary/test','grader','deadline',JSON.parse(fs.readFileSync(${JSON.stringify(isolation)},'utf8')),false,8000);
if(attempt.child) throw new Error('Runtime started before workspace binding');
try {const response=await attempt.callApi('SYNTHETIC_TIMEOUT');fs.writeFileSync(${JSON.stringify(path.join(root, 'deadline.json'))},JSON.stringify(response),{mode:0o600});} finally {await attempt.close();}
`, { mode: 0o600 })
    // A fresh root avoids the deliberate run-level blocker from the denial check.
    const deadlineRoot = path.join(root, 'deadline-run')
    fs.mkdirSync(deadlineRoot, { mode: 0o700 })
    const deadline = spawn(process.execPath, [deadlineDriver], { env: { ...cleanEnvironment({ root: deadlineRoot }), OTEL_EVAL_RUN_DIR: deadlineRoot, PROMPTFOO_DISABLE_TELEMETRY: '1', PROMPTFOO_CONFIG_DIR: path.join(deadlineRoot, 'promptfoo-state') }, stdio: ['ignore', 'pipe', 'pipe'] })
    let deadlineLog = ''
    deadline.stdout.on('data', chunk => { deadlineLog += chunk })
    deadline.stderr.on('data', chunk => { deadlineLog += chunk })
    const deadlineTimer = setTimeout(() => deadline.kill('SIGTERM'), 55000)
    const [deadlineCode] = await once(deadline, 'close')
    clearTimeout(deadlineTimer)
    fs.writeFileSync(path.join(root, 'deadline-driver.log'), deadlineLog, { mode: 0o600 })
    assert.equal(deadlineCode, 0, deadlineLog)
    const deadlineResponse = JSON.parse(fs.readFileSync(path.join(root, 'deadline.json'), 'utf8'))
    assert.equal(deadlineResponse.metadata.timedOut, true)
    assert.match(deadlineResponse.error, /timed out/)
    const graderRuntime = JSON.parse(fs.readFileSync(path.join(deadlineResponse.metadata.evidence, 'runtime.json'), 'utf8'))
    assert.equal(graderRuntime.tmpdir, path.join(deadlineResponse.metadata.workspace, 'tmp'))
    assert.equal(deadlineResponse.metadata.workspace, deadlineResponse.metadata.archivedWorkspace)
    const deadlinePID = Number(fs.readFileSync(path.join(deadlineResponse.metadata.evidence, 'process.pid'), 'utf8'))
    assert.throws(() => process.kill(deadlinePID, 0))
    assert.ok(fs.existsSync(path.join(deadlineResponse.metadata.evidence, 'native-events.jsonl')))
    console.log('Native synthetic evidence: ' + root)
  } finally {
    service.closeAllConnections()
    await new Promise<void>(resolve => service.close(() => resolve()))
  }
})

test('native setup rejects unavailable model, missing selected authentication and wrong version before inference', async () => {
  const { providerFixture } = await import('./test-fixture.ts')
  const { NativeAttempt } = await import('./native-lifecycle.ts')
  for (const scenario of ['model catalogue', 'authentication', 'version', 'private profile']) {
    const { root, isolation } = providerFixture()
    fs.writeFileSync(path.join(root, 'stub-settings.json'), JSON.stringify(scenario === 'model catalogue' ? { models: 'openai/absent' } : scenario === 'version' ? { version: '0.0.0' } : {}))
    const settings = scenario === 'authentication' ? { ...isolation, authFile: undefined } : scenario === 'private profile' ? { ...isolation, permission: undefined } : isolation
    await assert.rejects(NativeAttempt.create('openai/gpt-6.1-sol', 'grader', 'setup', settings, false), /setup failed/)
    const runs = fs.readdirSync(path.join(root, 'runs'))
    assert.equal(runs.length, 1)
    const failure = JSON.parse(fs.readFileSync(path.join(root, 'runs', runs[0], 'failure.json'), 'utf8'))
    assert.equal(failure.phase, scenario)
    assert.ok(!fs.existsSync(path.join(root, 'runs', runs[0], 'process.pid')))
    assert.ok(!JSON.stringify(failure).includes('synthetic-selected-secret'))
  }
})

test('deferred native startup retains setup failures and waits for owned processes', async () => {
  const { providerFixture } = await import('./test-fixture.ts')
  const { NativeAttempt } = await import('./native-lifecycle.ts')
  for (const scenario of ['unbound copy', 'missing workspace', 'server startup', 'credential prompt', 'expired deadline', 'unused attempt']) {
    const { root, binary, isolation } = providerFixture()
    const attempt = await NativeAttempt.create('openai/gpt-6.1-sol', 'grader', scenario, isolation, scenario === 'unbound copy')
    assert.equal(attempt.child, undefined)
    assert.equal(attempt.native, undefined)
    if (scenario === 'unused attempt') {
      await attempt.close()
      assert.ok(!fs.existsSync(path.join(attempt.evidence, 'process.pid')))
      continue
    }
    if (scenario === 'server startup') fs.writeFileSync(binary, `#!${process.execPath}\nprocess.exit(2);\n`, { mode: 0o700 })
    if (scenario === 'expired deadline') attempt.started -= attempt.timeoutMs
    try {
      const response = await attempt.callApi(scenario === 'credential prompt' ? 'synthetic-selected-secret' : 'synthetic setup', scenario === 'missing workspace' ? { prompt: { config: { working_dir: path.join(root, 'missing') } } } : undefined)
      assert.ok(response.error)
      const failure = JSON.parse(fs.readFileSync(path.join(attempt.evidence, 'failure.json'), 'utf8'))
      if (scenario === 'unbound copy' || scenario === 'missing workspace') assert.equal(failure.phase, 'workspace binding')
      if (scenario === 'server startup') assert.equal(failure.phase, 'server startup')
      if (scenario === 'expired deadline') assert.equal(response.metadata?.timedOut, true)
      assert.ok(!fs.existsSync(path.join(attempt.evidence, 'prompt.txt')))
      assert.ok(!JSON.stringify(response).includes('synthetic-selected-secret'))
      assert.ok(!JSON.stringify(failure).includes('synthetic-selected-secret'))
    } finally {
      await attempt.close()
    }
    const pidFile = path.join(attempt.evidence, 'process.pid')
    if (fs.existsSync(pidFile)) assert.throws(() => process.kill(Number(fs.readFileSync(pidFile, 'utf8')), 0))
    else assert.equal(attempt.child, undefined)
  }
})
