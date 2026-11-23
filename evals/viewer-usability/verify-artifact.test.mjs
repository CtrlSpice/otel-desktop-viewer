import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { tmpdir } from 'node:os'
import { stageDataset, datasetSource } from './stage-dataset.ts'
import { CHECKOUT_TRACE } from './verify-export.ts'
import { evaluationConfig, fullRunPlan, isExecutionError } from './opencode-v2-run-plan.mjs'

async function nativeExport(t, source) {
  const root = fs.realpathSync(fs.mkdtempSync(path.join(tmpdir(), 'opencode/export-assertion-')))
  t.diagnostic(`Native export assertion evidence: ${root}`)
  const previous = process.env.OTEL_EVAL_RUN_DIR
  process.env.OTEL_EVAL_RUN_DIR = root
  process.env.PROMPTFOO_DISABLE_TELEMETRY = '1'
  process.env.PROMPTFOO_CONFIG_DIR = path.join(root, 'state')
  t.after(() => {
    if (previous === undefined) delete process.env.OTEL_EVAL_RUN_DIR
    else process.env.OTEL_EVAL_RUN_DIR = previous
  })
  stageDataset(datasetSource(), path.join(root, 'fixture'))
  const document = JSON.parse(fs.readFileSync(path.join(root, 'fixture/traces.json'), 'utf8'))
  document.resourceSpans = document.resourceSpans.flatMap(resource => {
    resource.scopeSpans = resource.scopeSpans.flatMap(scope => {
      scope.spans = scope.spans.filter(span => span.traceId === CHECKOUT_TRACE)
      return scope.spans.length ? [scope] : []
    })
    return resource.scopeSpans.length ? [resource] : []
  })
  const metadata = {}
  for (const kind of ['valid', 'missing']) {
    const evidence = path.join(root, 'runs/investigator-' + kind)
    const workspace = path.join(evidence, 'workspace')
    fs.mkdirSync(workspace, { recursive: true })
    metadata[kind] = { runtime: 'opencode-v2', role: 'investigator', evidence, workspace }
  }
  if (source) {
    const rows = JSON.parse(fs.readFileSync(path.join(source, 'promptfoo.json'), 'utf8')).results.results
    const row = rows.find(row => {
      const packet = path.join(row.response?.metadata?.evidence ?? '', 'grading-packet.json')
      return row.vars.task_id === 'trace-export' && fs.existsSync(packet) &&
        JSON.parse(fs.readFileSync(packet, 'utf8')).artifact?.pass
    })
    assert.ok(row, 'A completed export with a passing saved replay is required')
    const workspace = row.response.metadata.workspace
    const files = fs.readdirSync(workspace, { recursive: true, withFileTypes: true }).filter(entry => entry.isFile())
    for (const entry of files) {
      const file = path.join(entry.parentPath, entry.name)
      const target = path.join(metadata.valid.workspace, path.relative(workspace, file))
      fs.mkdirSync(path.dirname(target), { recursive: true })
      fs.copyFileSync(file, target, fs.constants.COPYFILE_EXCL)
    }
    t.diagnostic(`Copied real export workspace from ${row.response.metadata.evidence}; source evidence remains read-only`)
  } else {
    fs.writeFileSync(path.join(metadata.valid.workspace, 'trace.json'), JSON.stringify(document))
  }
  let binary = process.env.OTEL_EVAL_VIEWER_BINARY
  if (!binary) {
    binary = path.join(root, 'stub-viewer')
    fs.writeFileSync(binary, `#!${process.execPath}
const http = require('node:http');
const args = process.argv.slice(2), port = flag => Number(args[args.indexOf(flag)+1]);
const rpc = http.createServer(async (req,res) => {
  if (req.url.startsWith('/export/')) return res.end(${JSON.stringify(JSON.stringify(document))});
  let body = ''; for await (const chunk of req) body += chunk;
  const request = JSON.parse(body);
  const result = request.method === 'getStats' ? {rejections:[]} :
    {columns:[{name:'count'}],rows:request.params.sql === 'select 1' ? [[1]] : [[6,0,0]],truncated:false};
  res.end(JSON.stringify({jsonrpc:'2.0',id:1,result}));
});
const receiver = http.createServer(async (req,res) => { for await (const chunk of req) {} res.end('{}'); });
rpc.listen(port('--browser-port'),'127.0.0.1'); receiver.listen(port('--http'),'127.0.0.1');
process.on('SIGTERM',()=>{rpc.close();receiver.close();rpc.closeAllConnections();receiver.closeAllConnections();});
`, { mode: 0o700 })
  }
  fs.writeFileSync(path.join(root, 'connection.json'), JSON.stringify({ binary }))
  const configured = evaluationConfig({ suite: import.meta.dirname, root, spec: fullRunPlan() })
  const task = configured.tests.find(task => task.vars.task_id === 'trace-export')
  assert.equal(task.assert[0].value, 'file://' + path.join(import.meta.dirname, 'verify-artifact.ts'))
  const scenarios = ['both', 'file-only', 'handoff-only', 'neither', 'grader-error']
  const { evaluate } = await import('promptfoo')
  await evaluate({
    prompts: ['{{scenario}}'],
    providers: [{ id: () => 'offline-export-investigator', async callApi(scenario) {
      return { output: scenario, metadata: metadata[['both', 'file-only', 'grader-error'].includes(scenario) ? 'valid' : 'missing'] }
    } }],
    defaultTest: {
      assert: [{ type: 'llm-rubric', value: 'Recorded export handoff criterion', threshold: 1 }],
      options: { provider: { id: () => 'offline-export-grader', async callApi(prompt) {
        if (prompt.includes('grader-error')) return { error: 'Offline grader unavailable' }
        const pass = prompt.includes('both') || prompt.includes('handoff-only')
        return { output: JSON.stringify({ pass, score: pass ? 1 : 0.5, reason: 'Recorded handoff judgment' }) }
      } } },
    },
    tests: scenarios.map(scenario => ({ ...task, vars: { ...task.vars, scenario } })),
    outputPath: path.join(root, 'promptfoo.json'),
  }, { maxConcurrency: 1, cache: false, share: false })
  const rows = JSON.parse(fs.readFileSync(path.join(root, 'promptfoo.json'), 'utf8')).results.results
  assert.equal(rows.length, scenarios.length)
  for (const row of rows) {
    const scenario = row.vars.scenario
    assert.equal(row.score, Number(scenario === 'both'), scenario)
    assert.equal(row.success, scenario === 'both', scenario)
    assert.equal(row.gradingResult.componentResults.length, 2)
    const artifact = row.gradingResult.componentResults.find(component => component.assertion.metric === 'export-file')
    const pass = ['both', 'file-only', 'grader-error'].includes(scenario)
    assert.equal(artifact.pass, pass, artifact.reason)
    assert.equal(artifact.score, Number(pass))
    assert.match(artifact.reason, pass ? /replayed unchanged/ : /No file in this investigator scratch/)
    const judge = row.gradingResult.componentResults.find(component => component.assertion.type === 'llm-rubric')
    assert.equal(judge.score, ['file-only', 'neither'].includes(scenario) ? 0.5 : scenario === 'grader-error' ? 0 : 1)
    assert.equal(isExecutionError(row), scenario === 'grader-error')
  }
  const replays = fs.readdirSync(root).filter(name => name.startsWith('export-replay-'))
  assert.ok(replays.length >= 3)
  for (const replay of replays) {
    const directory = path.join(root, replay)
    assert.equal(JSON.parse(fs.readFileSync(path.join(directory, 'teardown.json'), 'utf8')).status, 'removed')
    assert.equal(fs.existsSync(path.join(directory, 'otel-desktop-viewer')), false)
    const shutdown = JSON.parse(fs.readFileSync(path.join(directory, 'owned-shutdown.json'), 'utf8'))
    assert.equal(shutdown.waited, true)
    assert.throws(() => process.kill(shutdown.pid, 0), /ESRCH/)
  }
}

test('native Promptfoo loads the export file-function and calculates both components without replacing the assertion', t => nativeExport(t))
test('native Promptfoo verifies copied real export evidence using an owned viewer',
  { skip: !process.env.OTEL_EVAL_EXPORT_EVIDENCE_ROOT || !process.env.OTEL_EVAL_VIEWER_BINARY },
  t => nativeExport(t, process.env.OTEL_EVAL_EXPORT_EVIDENCE_ROOT))
