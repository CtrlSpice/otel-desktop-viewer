import test from 'node:test'
import assert from 'node:assert/strict'
import { spawn, type ChildProcess } from 'node:child_process'
import { once } from 'node:events'
import { runCancellation, stopOwnedGroup } from './run-cancellation.ts'

function gone(pid: number) {
  assert.throws(() => process.kill(pid, 0), /ESRCH/)
}

async function cleanup(child: ChildProcess) {
  if (child.pid && child.exitCode === null && child.signalCode === null) await stopOwnedGroup(child.pid, child, 100)
}

for (const signal of ['SIGINT', 'SIGTERM'] as const) {
  test(`shared cancellation handles ${signal} and removes its signal handlers`, { timeout: 15000 }, async t => {
    const module = new URL('./run-cancellation.ts', import.meta.url).href
    const child = spawn(process.execPath, ['--input-type=module', '-e', `
import assert from 'node:assert/strict';
import {runCancellation} from ${JSON.stringify(module)};
const before=['SIGINT','SIGTERM'].map(signal=>process.listenerCount(signal));
const keepAlive=setInterval(()=>{},1000);
const cancellation=runCancellation();
cancellation.signal.addEventListener('abort',()=>{
  process.send({aborted:cancellation.signal.aborted,reason:cancellation.signal.reason.message});
  cancellation.dispose();
  assert.deepEqual(['SIGINT','SIGTERM'].map(signal=>process.listenerCount(signal)),before);
  clearInterval(keepAlive);
  process.disconnect();
},{once:true});
process.send('ready');
`], { detached: true, stdio: ['ignore', 'ignore', 'pipe', 'ipc'] })
    t.after(() => cleanup(child))
    let stderr = ''
    child.stderr!.on('data', chunk => { stderr += chunk })
    await once(child, 'message')
    const aborted = once(child, 'message')
    const closed = once(child, 'close')
    child.kill(signal)
    assert.deepEqual((await aborted)[0], { aborted: true, reason: `Evaluation cancelled by ${signal}` })
    const [code, exitSignal] = await closed
    assert.equal(code, 0, stderr)
    assert.equal(exitSignal, null)
    gone(child.pid!)
  })
}

test('shared cancellation inherits existing and later parent aborts and supports explicit cancellation', () => {
  for (const alreadyAborted of [false, true]) {
    const parent = new AbortController()
    const reason = new Error('parent cancelled')
    if (alreadyAborted) parent.abort(reason)
    const cancellation = runCancellation(parent.signal)
    try {
      if (!alreadyAborted) parent.abort(reason)
      assert.equal(cancellation.signal.aborted, true)
      assert.equal(cancellation.signal.reason, reason)
    } finally { cancellation.dispose() }
  }
  const cancellation = runCancellation()
  const reason = new Error('explicit cancellation')
  try {
    cancellation.cancel(reason)
    assert.equal(cancellation.signal.reason, reason)
  } finally { cancellation.dispose() }
})

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
