import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { spawn, type ChildProcess } from 'node:child_process'
import { once } from 'node:events'
import { setTimeout as delay } from 'node:timers/promises'
import { pathToFileURL } from 'node:url'
import { TEMPORARY, SUITE, readJson, record } from './runtime.ts'
import { cleanEnvironment } from './clean-environment.ts'
import { stopOwnedGroup } from './run-cancellation.ts'
import { providerFixture } from './test-fixture.ts'
import { pricingSnapshot } from './pricing.ts'
import apiPrices from './api-prices.json' with { type: 'json' }

const pricing = pricingSnapshot({ capturedAt: '2026-10-08T06:00:00Z', api: apiPrices,
  exchange: { source: 'synthetic-rate', date: '2026-10-07', cadPerUsd: 1.4257 } })

async function waitFor(check: () => boolean) {
  const deadline = performance.now() + 10000
  while (!check()) {
    if (performance.now() >= deadline) throw new Error('Scripted process did not become ready')
    await delay(10)
  }
}

function gone(pid: number) {
  assert.throws(() => process.kill(pid, 0), /ESRCH/)
}

async function cleanup(child: ChildProcess) {
  if (child.pid && child.exitCode === null && child.signalCode === null) await stopOwnedGroup(child.pid, child, 100)
}

for (const signal of ['SIGINT', 'SIGTERM'] as const) {
  test(`pilot ${signal} awaits its worker, detached native attempt and viewer; preserves a user process`, { timeout: 15000 }, async t => {
    const root = fs.mkdtempSync(path.join(TEMPORARY, 'eval-pilot-cancel-'))
    const suite = path.join(root, 'suite')
    fs.mkdirSync(suite)
    fs.writeFileSync(path.join(suite, 'native-evaluate.ts'), `
const fs=require('node:fs'),{spawn}=require('node:child_process');
const native=spawn(process.execPath,['-e',"process.send('ready');setInterval(()=>{},1000)"],{detached:true,stdio:['ignore','ignore','ignore','ipc']});
native.once('message',()=>{
  process.send({type:'otel-eval-native-start',runnerPID:process.pid,root:fs.realpathSync('.'),sessionPID:native.pid},()=>{
    fs.writeFileSync('ready.json',JSON.stringify({worker:process.pid,native:native.pid}));
  });
});
setInterval(()=>fs.appendFileSync('scheduled.txt','call\\n'),20);
`)
    const driver = path.join(root, 'driver.mjs')
    fs.writeFileSync(driver, `
import fs from 'node:fs';import {spawn} from 'node:child_process';import {once} from 'node:events';
import {runPilot} from ${JSON.stringify(path.join(SUITE, 'run-pilot.ts'))};
import {ownedViewerStop} from ${JSON.stringify(path.join(SUITE, 'owned-viewer-stop.ts'))};
const viewer=spawn(process.execPath,['-e',"process.on('SIGTERM',()=>setTimeout(()=>process.exit(0),150));process.send('ready');setInterval(()=>{},1000)"],{detached:true,stdio:['ignore','ignore','ignore','ipc']});
const closed=new Promise(resolve=>viewer.once('close',(code,signal)=>resolve({code,signal})));
await once(viewer,'message');fs.writeFileSync(${JSON.stringify(path.join(root, 'viewer.pid'))},String(viewer.pid));
process.exitCode=await runPilot(${JSON.stringify(root)},${JSON.stringify(suite)},${JSON.stringify(pricing)},ownedViewerStop(${JSON.stringify(root)},{process:viewer,pid:viewer.pid,closed}));
`)
    const user = spawn(process.execPath, ['-e', 'setInterval(()=>{},1000)'], { detached: true, stdio: 'ignore' })
    t.after(() => cleanup(user))
    const parent = spawn(process.execPath, [driver], { detached: true, env: cleanEnvironment({ root, inherited: {} }), stdio: ['ignore', 'pipe', 'pipe'] })
    t.after(async () => {
      await cleanup(parent)
      for (const file of ['ready.json', 'viewer.pid']) {
        if (!fs.existsSync(path.join(root, file))) continue
        const pids = file === 'ready.json' ? Object.values(record(readJson(path.join(root, file)))) : [Number(fs.readFileSync(path.join(root, file), 'utf8'))]
        for (const pid of pids) await stopOwnedGroup(Number(pid), undefined, 100)
      }
    })
    let stderr = ''
    parent.stderr!.on('data', chunk => { stderr += chunk })
    const closed = once(parent, 'close')
    await waitFor(() => fs.existsSync(path.join(root, 'ready.json')))
    const pids = record(readJson(path.join(root, 'ready.json')))
    parent.kill(signal)
    await delay(30)
    parent.kill(signal)
    const [code, exitSignal] = await closed
    fs.writeFileSync(path.join(root, 'driver.stderr'), stderr)
    assert.equal(code, 1, stderr)
    assert.equal(exitSignal, null)
    for (const pid of Object.values(pids)) gone(Number(pid))
    gone(Number(fs.readFileSync(path.join(root, 'viewer.pid'), 'utf8')))
    assert.equal(process.kill(user.pid!, 0), true)
    assert.ok(fs.existsSync(path.join(root, 'owned-shutdown.json')), stderr)
    assert.equal(record(readJson(path.join(root, 'owned-shutdown.json'))).waited, true)
    assert.equal(record(readJson(path.join(root, 'pilot-completion.json'))).status, 'cancelled')
    const scheduled = fs.existsSync(path.join(root, 'scheduled.txt')) ? fs.readFileSync(path.join(root, 'scheduled.txt'), 'utf8') : ''
    await delay(60)
    assert.equal(fs.existsSync(path.join(root, 'scheduled.txt')) ? fs.readFileSync(path.join(root, 'scheduled.txt'), 'utf8') : '', scheduled)
  })

}

for (const signal of ['SIGINT', 'SIGTERM', 'disconnect'] as const) {
  test(`native worker ${signal} aborts scheduling and closes an active scripted native server`, { timeout: 15000 }, async t => {
    const { root, isolation } = providerFixture()
    fs.writeFileSync(path.join(root, 'connection.json'), '{}')
    const lifecycle = JSON.stringify(pathToFileURL(path.join(SUITE, 'native-lifecycle.ts')).href)
    // Replace only external libraries. Exercise the actual scheduler, attempt
    // lifecycle, private stub executable, signal handlers and process cleanup.
    const promptfoo = `
import fs from 'node:fs';
export async function loadApiProvider(){return {callApi:async(_prompt,_context,{abortSignal})=>{
  process.send({type:'synthetic-inference-ready'});
  await new Promise((resolve,reject)=>{abortSignal.addEventListener('abort',()=>reject(new Error('cancelled')),{once:true});if(abortSignal.aborted)reject(new Error('cancelled'));});
}};}
export async function evaluate(suite,options){
 const {NativeAttempt}=await import(${lifecycle});const results=[];
 for(const [testIdx,test] of suite.tests.entries()){
  if(options.abortSignal.aborted)break;
  fs.appendFileSync(${JSON.stringify(path.join(root, 'calls.txt'))},'call\\n');
  const attempt=await NativeAttempt.create(test.vars.eval_model,'grader',test.vars.task_id,${JSON.stringify(isolation)},false);
  try {const response=await attempt.callApi('scripted call');results.push({testIdx,vars:test.vars,response});}
  finally {await attempt.close();}
  options.progressCallback();
 }
 return {toEvaluateSummary:async()=>({results})};
}`
    const otel = 'export const context={},propagation={},trace={},ROOT_CONTEXT={},SpanStatusCode={},isSpanContextValid=()=>false;'
    const driver = path.join(root, 'driver.mjs')
    fs.writeFileSync(driver, `
import {registerHooks} from 'node:module';
const modules=${JSON.stringify({ promptfoo, '@opentelemetry/api': otel })};
registerHooks({resolve(specifier,context,next){return specifier in modules ? {url:'data:text/javascript,'+encodeURIComponent(modules[specifier]),shortCircuit:true}:next(specifier,context);}});
const {runNativeEvaluation}=await import(${JSON.stringify(path.join(SUITE, 'native-evaluate.ts'))});
process.exitCode=await runNativeEvaluation({providers:['echo'],prompts:['scripted'],tests:[0,1,2].map(i=>({vars:{eval_model:'openai/gpt-6.1-sol',task_id:String(i)}}))},1);
`)
    const child = spawn(process.execPath, [driver], { detached: true,
      env: { ...cleanEnvironment({ root, inherited: {} }), OTEL_EVAL_RUN_DIR: root }, stdio: ['ignore', 'pipe', 'pipe', 'ipc'] })
    let stderr = ''
    child.stderr!.on('data', chunk => { stderr += chunk })
    let ready = false
    const native = new Set<number>()
    child.on('message', message => {
      const event = record(message)
      if (event.type === 'otel-eval-native-start') native.add(Number(event.sessionPID))
      if (event.type === 'synthetic-inference-ready') ready = true
    })
    t.after(async () => {
      fs.writeFileSync(path.join(root, 'driver.stderr'), stderr)
      await cleanup(child)
      for (const pid of native) await stopOwnedGroup(pid, undefined, 100)
    })
    // With the parent IPC endpoint explicitly disconnected, wait for process
    // exit rather than Node's aggregate pipe/IPC close notification.
    const closed = once(child, signal === 'disconnect' ? 'exit' : 'close')
    await waitFor(() => ready || child.exitCode !== null)
    assert.equal(ready, true, stderr)
    if (signal === 'disconnect') child.disconnect()
    else child.kill(signal)
    const [code, exitSignal] = await closed
    assert.equal(code, 1, stderr)
    assert.equal(exitSignal, null)
    assert.equal(native.size, 1)
    for (const pid of native) gone(pid)
    assert.equal(fs.readFileSync(path.join(root, 'calls.txt'), 'utf8'), 'call\n')
    const result = record(record(readJson(path.join(root, 'pilot.json'))).results)
    assert.equal((result.results as unknown[]).length, 1)
    assert.equal((record(readJson(path.join(root, 'deferred-slots.json'))).slots as unknown[]).length, 2)
  })
}

test('owned process shutdown escalates SIGTERM-resistant processes and waits for exit', { timeout: 5000 }, async t => {
  const child = spawn(process.execPath, ['-e', "process.on('SIGTERM',()=>{});process.send('ready');setInterval(()=>{},1000)"], { detached: true, stdio: ['ignore', 'ignore', 'ignore', 'ipc'] })
  t.after(() => cleanup(child))
  await once(child, 'message')
  await stopOwnedGroup(child.pid!, child, 50)
  assert.equal(child.signalCode, 'SIGKILL')
  gone(child.pid!)
})

test('owned process shutdown waits for resistant descendants after the group leader exits', { timeout: 5000 }, async t => {
  const child = spawn(process.execPath, ['-e', `
const {spawn}=require('node:child_process');
const descendant=spawn(process.execPath,['-e',"process.on('SIGTERM',()=>{});process.send('ready');setInterval(()=>{},1000)"],{stdio:['ignore','ignore','ignore','ipc']});
descendant.once('message',()=>process.send(descendant.pid));
setInterval(()=>{},1000);
`], { detached: true, stdio: ['ignore', 'ignore', 'ignore', 'ipc'] })
  t.after(() => stopOwnedGroup(child.pid!, child, 100))
  const [descendant] = await once(child, 'message')
  await stopOwnedGroup(child.pid!, child, 50)
  gone(child.pid!)
  gone(Number(descendant))
})
