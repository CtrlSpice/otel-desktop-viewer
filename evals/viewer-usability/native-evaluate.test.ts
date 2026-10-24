import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { spawnSync } from 'node:child_process'
import { SUITE, TEMPORARY } from './runtime.ts'
import { cleanEnvironment } from './clean-environment.ts'

test('native evaluation stops before another provider call on investigator or grader execution failure', () => {
  for (const scenario of ['investigator-error', 'grader-error', 'blocking-permission', 'valid-scores']) {
    const root = fs.mkdtempSync(path.join(TEMPORARY, 'eval-stop-on-error-'))
    fs.chmodSync(root, 0o700)
    fs.writeFileSync(path.join(root, 'connection.json'), '{}', { mode: 0o600 })
    const driver = path.join(root, 'driver.mjs')
    fs.writeFileSync(driver, `
import fs from 'node:fs';
import {runNativeEvaluation} from ${JSON.stringify(path.join(SUITE, 'native-evaluate.ts'))};
import grade from ${JSON.stringify(path.join(SUITE, 'grade.ts'))};
let calls=0;
const scenario=${JSON.stringify(scenario)};
const provider={id:()=> 'canary/test',callApi:async()=>{
  calls++;
  fs.writeFileSync(${JSON.stringify(path.join(root, 'calls.json'))},JSON.stringify(calls));
  if(scenario==='blocking-permission')fs.writeFileSync(${JSON.stringify(path.join(root, 'harness-blocker.json'))},'{}');
  return scenario==='blocking-permission'
    ? {error:'synthetic blocked viewer command',metadata:{model:'canary/test',executionFailure:'model'}}
    : scenario==='investigator-error'
    ? {error:'synthetic investigator error',metadata:{model:'canary/test'}}
    : {output:'synthetic response',metadata:{model:'canary/test'}};
}};
const suite={prompts:['synthetic task'],providers:[provider],
  extensions:[${JSON.stringify('file://' + path.join(SUITE, 'native-hooks.mjs') + ':afterEach')}],
  tests:[0,0.5,1].map(score=>({vars:{eval_model:'canary/test',task_id:String(score)},assert:[{
    type:'javascript',value:()=>{if(scenario==='grader-error')return grade('synthetic response',{vars:{task_id:String(score)},providerResponse:{metadata:{model:'canary/test'}}});return {pass:score===1,score,reason:'synthetic score'};}
  }]}))};
process.exitCode=await runNativeEvaluation(suite,1);
`, { flag: 'wx', mode: 0o600 })
    const child = spawnSync(process.execPath, [driver], {
      cwd: root,
      env: { ...cleanEnvironment({ root }), OTEL_EVAL_RUN_DIR: root,
        PROMPTFOO_DISABLE_TELEMETRY: '1', PROMPTFOO_CONFIG_DIR: path.join(root, 'promptfoo-state') },
      encoding: 'utf8', timeout: 30000,
    })
    assert.equal(child.status, scenario === 'valid-scores' ? 0 : 1, child.stderr)
    const rows = JSON.parse(fs.readFileSync(path.join(root, 'pilot.json'), 'utf8')).results.results
    const calls = JSON.parse(fs.readFileSync(path.join(root, 'calls.json'), 'utf8'))
    assert.equal(calls, scenario === 'valid-scores' ? 3 : 1)
    assert.equal(rows.length, calls)
    assert.equal(fs.existsSync(path.join(root, 'execution-error.json')), scenario !== 'valid-scores')
    if (scenario === 'valid-scores') assert.deepEqual(rows.map((row: { score: number }) => row.score), [0, 0.5, 1])
    else assert.equal(rows[0].failureReason, 2)
  }
})

test('model errors pause only that model, preserve repeated slots, and export later batch traces', () => {
  const root = fs.mkdtempSync(path.join(TEMPORARY, 'eval-model-pause-'))
  fs.chmodSync(root, 0o700)
  fs.writeFileSync(path.join(root, 'connection.json'), '{}', { mode: 0o600 })
  const driver = path.join(root, 'driver.mjs')
  fs.writeFileSync(driver, `
import fs from 'node:fs';
import http from 'node:http';
import {once} from 'node:events';
import {runNativeEvaluation} from ${JSON.stringify(path.join(SUITE, 'native-evaluate.ts'))};
const calls=[], exports=[];
const server=http.createServer(async(req,res)=>{const chunks=[];for await(const chunk of req)chunks.push(chunk);exports.push(Buffer.concat(chunks));res.writeHead(200,{'Content-Type':'application/json'}).end('{}');});
server.listen(0,'127.0.0.1');await once(server,'listening');
process.env.OTEL_EVAL_OBSERVER_HTTP='http://127.0.0.1:'+server.address().port;
const provider={id:()=> 'canary/test',callApi:async(_prompt,context)=>{
  const model=context.vars.eval_model, task=context.vars.task_id;
  calls.push({model,task});
  return model!=='canary/healthy' && task==='first'
    ? {error:'synthetic HTTP403 endpoint unavailable',metadata:{model,executionFailure:'model'}}
    : {output:'synthetic response',metadata:{model}};
}};
const suite={prompts:['synthetic {{task_id}}'],providers:[provider],
  extensions:[${JSON.stringify('file://' + path.join(SUITE, 'native-hooks.mjs') + ':afterEach')}],
  tests:['first','after-pause-canary'].flatMap(task=>['canary/broken','canary/healthy','canary/also-broken'].map(model=>({description:task+' '+model,vars:{eval_model:model,task_id:task},assert:[{type:'javascript',value:()=>({pass:false,score:0.5,reason:'synthetic partial'})}]})))};
try {process.exitCode=await runNativeEvaluation(suite,2);}
finally {fs.writeFileSync(${JSON.stringify(path.join(root, 'calls.json'))},JSON.stringify(calls));fs.writeFileSync(${JSON.stringify(path.join(root, 'exports.bin'))},Buffer.concat(exports));await new Promise(resolve=>server.close(resolve));}
`, { flag: 'wx', mode: 0o600 })
  const child = spawnSync(process.execPath, [driver], {
    cwd: root,
    env: { ...cleanEnvironment({ root }), OTEL_EVAL_RUN_DIR: root,
      PROMPTFOO_DISABLE_TELEMETRY: '1', PROMPTFOO_CONFIG_DIR: path.join(root, 'promptfoo-state') },
    encoding: 'utf8', timeout: 60000,
  })
  assert.equal(child.status, 1, child.stderr)
  const calls = JSON.parse(fs.readFileSync(path.join(root, 'calls.json'), 'utf8'))
  assert.equal(calls.length, 6, child.stderr)
  assert.equal(calls.filter((call: { model: string }) => call.model === 'canary/healthy').length, 4)
  const rows = JSON.parse(fs.readFileSync(path.join(root, 'pilot.json'), 'utf8')).results.results
  assert.equal(rows.length, 6)
  assert.equal(rows.filter((row: { failureReason: number }) => row.failureReason === 2).length, 2)
  assert.equal(rows.filter((row: { score: number }) => row.score === 0.5).length, 4)
  assert.equal(new Set(rows.map((row: { testIdx: number; repeatIndex: number }) => `${row.testIdx}/${row.repeatIndex}`)).size, 6)
  const errors = JSON.parse(fs.readFileSync(path.join(root, 'model-errors.json'), 'utf8'))
  assert.deepEqual(errors.map((error: { model: string }) => error.model), ['canary/broken', 'canary/also-broken'])
  assert.equal(fs.existsSync(path.join(root, 'execution-error.json')), false)
  const deferred = JSON.parse(fs.readFileSync(path.join(root, 'deferred-slots.json'), 'utf8'))
  assert.equal(deferred.slots.length, 6)
  assert.ok(deferred.slots.every((slot: { model: string }) => slot.model !== 'canary/healthy'))
  const telemetry = fs.readFileSync(path.join(root, 'exports.bin')).toString()
  for (const row of rows) assert.ok(telemetry.includes(row.traceId), `Missing exported test-case trace ${row.traceId}`)
})
