import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { spawn } from 'node:child_process'
import { once } from 'node:events'
import { setTimeout as delay } from 'node:timers/promises'
import { SUITE, TEMPORARY, readJson, record } from './runtime.ts'
import { cleanEnvironment } from './clean-environment.ts'
import { stopOwnedGroup } from './run-cancellation.ts'
import { pricingSnapshot } from './pricing.ts'
import apiPrices from './api-prices.json' with { type: 'json' }

for (const phase of ['preparation', 'verification', 'evaluation', 'shutdown']) {
  for (const signal of ['SIGINT', 'SIGTERM'] as const) {
    test(`overnight ${signal} during ${phase} awaits owned shutdown and exits nonzero`, { timeout: 15000 }, async t => {
      const root = fs.mkdtempSync(path.join(TEMPORARY, 'eval-overnight-cancel-'))
      const suite = path.join(root, 'suite')
      fs.mkdirSync(suite)
      // Actual runner and viewer lifecycle; only model/grading and verification
      // inputs are scripted. No native model executable or external service.
      for (const file of ['run-overnight.ts', 'run-pilot.ts', 'run-cancellation.ts', 'prepare.ts',
        'owned-viewer-stop.ts', 'stage-dataset.ts', 'runtime.ts', 'pricing.ts', 'model-summary.ts',
        'clean-environment.ts', 'openai-models.json', 'tasks.json']) {
        fs.copyFileSync(path.join(SUITE, file), path.join(suite, file))
      }
      fs.writeFileSync(path.join(suite, 'package.json'), '{"type":"module"}')
      fs.writeFileSync(path.join(suite, 'grade.ts'), 'export const GRADER_MODEL="synthetic"')
      fs.writeFileSync(path.join(suite, 'verify.ts'), `
import fs from 'node:fs';
export async function verify(){
 if(${JSON.stringify(phase)}==='verification'){
  fs.writeFileSync(process.env.OTEL_EVAL_RUN_DIR+'/phase-ready','');
  await new Promise(resolve=>setTimeout(resolve,300));
 }
}`)
      fs.writeFileSync(path.join(suite, 'native-evaluate.ts'), `
import fs from 'node:fs';
fs.writeFileSync('worker.pid',String(process.pid));
if(${JSON.stringify(phase)}!=='shutdown'){fs.writeFileSync('phase-ready','');setInterval(()=>{},1000);}
`)
      const binary = path.join(root, 'viewer-stub')
      fs.writeFileSync(binary, `#!${process.execPath}
const fs=require('node:fs'),http=require('node:http');
const root=${JSON.stringify(root)},phase=${JSON.stringify(phase)};
const port=flag=>Number(process.argv[process.argv.indexOf(flag)+1]);
const rpc=http.createServer(async(req,res)=>{
 if(phase==='preparation'){fs.writeFileSync(root+'/phase-ready','');return;}
 let body='';for await(const chunk of req)body+=chunk;
 const request=JSON.parse(body);
 res.end(JSON.stringify({result:request.method==='getStats'?{rejections:[]}:{columns:[],rows:request.params.sql==='SELECT 1'?[[1]]:[[0,0,0]],truncated:false}}));
});
const ingest=http.createServer((req,res)=>{req.resume();res.end('{}');});
rpc.listen(port('--browser-port'),'127.0.0.1');ingest.listen(port('--http'),'127.0.0.1');
process.on('SIGTERM',()=>{if(phase==='shutdown')fs.writeFileSync(root+'/phase-ready','');setTimeout(()=>{rpc.closeAllConnections();ingest.closeAllConnections();rpc.close();ingest.close();},100);});
`, { mode: 0o700 })
      const dataset = path.join(root, 'dataset')
      fs.mkdirSync(dataset)
      fs.writeFileSync(path.join(dataset, 'manifest.json'), JSON.stringify({ startTimeUnixNano: '1', endTimeUnixNano: '2', storedCounts: { spans: 0, logs: 0, datapoints: 0 }, requests: [{ signal: 'logs', file: 'logs.json' }] }))
      fs.writeFileSync(path.join(dataset, 'logs.json'), '{"resourceLogs":[]}')
      fs.writeFileSync(path.join(root, 'pricing-snapshot.json'), JSON.stringify(pricingSnapshot({ capturedAt: '2026-10-08T06:00:00Z', api: apiPrices, exchange: { source: 'synthetic', date: '2026-10-07', cadPerUsd: 1.4 } })))
      const child = spawn(process.execPath, [path.join(suite, 'run-overnight.ts')], {
        detached: true, stdio: ['ignore', 'pipe', 'pipe'],
        env: { ...cleanEnvironment({ root, inherited: {} }), OTEL_EVAL_RUN_DIR: root,
          OTEL_EVAL_VIEWER_BINARY: binary, OTLP_DATASET: dataset, OTEL_EVAL_ISOLATION_FILE: path.join(root, 'unused-isolation.json') },
      })
      t.after(async () => {
        if (child.pid && child.exitCode === null && child.signalCode === null) await stopOwnedGroup(child.pid, child, 100)
        for (const file of ['runtime/process.json', 'worker.pid']) {
          if (!fs.existsSync(path.join(root, file))) continue
          const pid = file.endsWith('.json') ? Number(record(readJson(path.join(root, file))).pid) : Number(fs.readFileSync(path.join(root, file), 'utf8'))
          await stopOwnedGroup(pid, undefined, 100)
        }
      })
      let stderr = ''
      child.stderr!.on('data', chunk => { stderr += chunk })
      const closed = once(child, 'close')
      const deadline = performance.now() + 10000
      while (!fs.existsSync(path.join(root, 'phase-ready')) && child.exitCode === null) {
        assert.ok(performance.now() < deadline, 'Scripted phase did not start: ' + stderr)
        await delay(10)
      }
      assert.equal(child.exitCode, null, stderr)
      child.kill(signal)
      const [code, exitSignal] = await closed
      fs.writeFileSync(path.join(root, 'driver.stderr'), stderr)
      assert.equal(code, 1, stderr)
      assert.equal(exitSignal, null)
      const pid = Number(record(readJson(path.join(root, 'runtime/process.json'))).pid)
      assert.throws(() => process.kill(pid, 0), /ESRCH/)
      if (phase === 'evaluation' || phase === 'shutdown') {
        const worker = Number(fs.readFileSync(path.join(root, 'worker.pid'), 'utf8'))
        assert.throws(() => process.kill(worker, 0), /ESRCH/)
        const completion = record(readJson(path.join(root, 'pilot-completion.json')))
        if (phase === 'evaluation') assert.equal(completion.status, 'cancelled')
        else assert.equal(completion.exit, 0)
      } else {
        assert.ok(!fs.existsSync(path.join(root, 'worker.pid')))
        assert.equal(record(readJson(path.join(root, 'overnight-failure.json'))).phase, phase)
      }
      if (phase !== 'preparation') assert.equal(record(readJson(path.join(root, 'owned-shutdown.json'))).waited, true)
    })
  }
}
