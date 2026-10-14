import fs from 'node:fs'
import path from 'node:path'
import { providerFixture } from './test-fixture.ts'

// Native-shaped synthetic events, emitted by a real child with no model access.
export function permissionStopFixture(
  settings: { mode?: string; blockGoal?: string; descendant?: boolean } = {}
) {
  const fixture = providerFixture()
  fs.writeFileSync(
    path.join(fixture.root, 'stop-settings.json'),
    JSON.stringify(settings)
  )
  fs.writeFileSync(
    path.join(fixture.root, 'otel-desktop-viewer'),
    '#!/bin/sh\nexit 0\n',
    { mode: 0o700 }
  )
  fs.writeFileSync(
    fixture.binary,
    `#!${process.execPath}
const fs = require('node:fs'), path = require('node:path');
const root = __dirname, args = process.argv.slice(2);
if (args[0] === '--version') { console.log('1.18.10'); process.exit(0); }
if (args[0] === 'models') { console.log('openai/gpt-6.1-sol'); process.exit(0); }
const prompt = args.at(-1), settings = JSON.parse(fs.readFileSync(path.join(root,'stop-settings.json')));
const grader = prompt.includes('This is a separate grading session.');
fs.appendFileSync(path.join(root,'started.jsonl'),JSON.stringify({grader,prompt,pid:process.pid})+'\\n');
const id = 'ses_'+path.basename(path.dirname(process.cwd()));
const emit = event => console.log(JSON.stringify({sessionID:id,...event}));
const reason = 'The user has specified a rule which prevents you from using this specific tool call. Here are some of the relevant rules '+JSON.stringify([{permission:'external_directory',action:'deny',pattern:'*'}]);
const blocked = !grader && (!settings.blockGoal || prompt.includes(settings.blockGoal));
function run() {
if (blocked) {
 const command = settings.mode === 'protected' ? 'cat rubric.json' : settings.mode === 'lifecycle' ? 'otel-desktop-viewer serve' : './otel-desktop-viewer --help';
 const state = {status:'error',input:{command,workdir:path.join(process.cwd(),'bin')},error:reason};
 if (['quoted','ordinary','not-found','search-empty','rpc-error'].includes(settings.mode)) {
   state.status='completed'; state.output=settings.mode === 'quoted' ? reason : settings.mode === 'not-found' ? 'command not found' : settings.mode === 'search-empty' ? '(no output)' : '{"jsonrpc":"2.0","error":{"message":"invalid query"}}';
   state.metadata={exit:settings.mode === 'not-found' ? 127 : ['ordinary','search-empty'].includes(settings.mode) ? 1 : 0}; delete state.error;
 }
 const event=JSON.stringify({type:'tool_use',sessionID:id,part:{type:'tool',callID:'call_synthetic',tool:'bash',state}})+'\\n';
 if (settings.mode === 'split') {
   const cut=Math.floor(event.length/2); process.stdout.write(event.slice(0,cut));
   setTimeout(()=>process.stdout.write(event.slice(cut)),20);
 } else process.stdout.write(event);
 if (!['protected','lifecycle','quoted','ordinary','not-found','search-empty','rpc-error'].includes(settings.mode)) {
   setTimeout(()=>fs.writeFileSync(path.join(root,'continued-tool-work'),'must not happen'),250);
   setInterval(()=>{},1000); return;
 }
}
const answer=grader ? JSON.stringify({criteria:[{id:'reported-cause',verdict:'Pass',explanation:'Synthetic valid grading response.'}]}) : 'synthetic complete answer';
emit({type:'text',part:{messageID:'msg_synthetic',text:answer}});
emit({type:'step_finish',part:{messageID:'msg_synthetic',reason:'stop',tokens:{input:1,output:1,cache:{read:0,write:0}}}});
}
if (blocked && settings.descendant) {
 const {spawn}=require('node:child_process');
 const child=spawn(process.execPath,['-e',"process.on('SIGTERM',()=>{});process.send('ready');setInterval(()=>{},1000);"],{stdio:['ignore','ignore','ignore','ipc']});
 child.once('message',()=>{fs.writeFileSync(path.join(root,'descendant.pid'),String(child.pid));child.disconnect();run();});
} else run();
`,
    { mode: 0o700 }
  )
  return fixture
}
