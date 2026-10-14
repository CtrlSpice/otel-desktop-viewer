import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { spawnSync } from 'node:child_process'
import { SUITE, TEMPORARY } from './runtime.ts'
import { cleanEnvironment } from './clean-environment.ts'

test('native evaluation stops before another provider call on investigator or grader execution failure', () => {
  for (const scenario of ['investigator-error', 'grader-error', 'valid-scores']) {
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
  return scenario==='investigator-error'
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
