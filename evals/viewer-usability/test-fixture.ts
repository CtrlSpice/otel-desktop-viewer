import fs from 'node:fs'
import path from 'node:path'
import { TEMPORARY } from './runtime.ts'

const stub = `#!${process.execPath}
const fs = require('node:fs');
const path = require('node:path');
const args = process.argv.slice(2);
const settings = JSON.parse(fs.readFileSync(path.join(__dirname,'stub-settings.json')));
if (args[0] === '--version') {console.log(settings.version || '1.18.10'); process.exit(0);}
if (args[0] === 'models') {console.log((args.includes('--refresh') && settings.refreshedModels) || settings.models || 'openai/gpt-6.1-sol'); process.exit(0);}
if (args[0] !== 'run') process.exit(2);
const auth = JSON.parse(fs.readFileSync(path.join(process.env.XDG_DATA_HOME,'opencode','auth.json')));
const secret = auth.openai.access;
const observation = {cwd:process.cwd(),args,env:process.env,
  config:JSON.parse(fs.readFileSync(path.join(process.env.OPENCODE_CONFIG_DIR,'opencode.json'))),
  emptyScratch:fs.readdirSync(process.cwd()).length === 0};
fs.writeFileSync(path.join(process.cwd(),'stub-observation.json'),JSON.stringify(observation));
fs.writeFileSync(path.join(process.env.XDG_DATA_HOME,'opencode','opencode.db'),'test session only');
process.stderr.write(secret.slice(0,6));
setTimeout(()=>{
  process.stderr.write(secret.slice(6));
  if (process.env.SYNTHETIC_API_KEY) process.stderr.write(process.env.SYNTHETIC_API_KEY);
  if (settings.tools) {
    for (const [command,status,exit] of [['otel-desktop-viewer logs --help','completed',0],['otel-desktop-viewer query SELECT','error',1]])
      console.log(JSON.stringify({type:'tool_use',part:{tool:'bash',state:{input:{command},status,metadata:{exit},output:'tool output'}}}));
    console.log(JSON.stringify({type:'text',part:{messageID:'old-message',text:'ignore previous answer'}}));
  }
  console.log(JSON.stringify({type:'text',sessionID:'ses_test_'+path.basename(path.dirname(process.env.HOME)),part:{messageID:'msg_test',text:settings.answer || 'test answer '+secret}}));
  process.stdout.write(JSON.stringify({type:'step_finish',sessionID:'ses_test_'+path.basename(path.dirname(process.env.HOME)),part:{messageID:'msg_test',reason:settings.incomplete ? 'length' : 'stop',tokens:settings.badTokens ? secret : {input:3,output:2}}}));
  process.exitCode = settings.incomplete ? 1 : 0;
},10);
`

export function providerFixture() {
  const root = fs.mkdtempSync(
    path.join(TEMPORARY, 'eval-isolation-adapter-test-')
  )
  fs.chmodSync(root, 0o700)
  const binary = path.join(root, 'opencode-stub')
  fs.writeFileSync(binary, stub, { mode: 0o700 })
  fs.writeFileSync(path.join(root, 'stub-settings.json'), '{}')
  const authFile = path.join(root, 'source-auth.json')
  fs.writeFileSync(
    authFile,
    JSON.stringify({
      openai: { type: 'oauth', access: 'synthetic-selected-secret' },
      other: { type: 'api', key: 'unselected-secret' },
    }),
    { mode: 0o600 }
  )
  const isolation = {
    opencodeBinary: binary,
    authFile,
    permission: {
      bash: { '*': 'allow', '*viewer serve*': 'deny' },
      task: 'deny',
    },
  }
  process.env.OTEL_EVAL_RUN_DIR = root
  return { root, binary, authFile, isolation }
}
