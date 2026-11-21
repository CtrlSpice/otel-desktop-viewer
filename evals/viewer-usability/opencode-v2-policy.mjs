import fs from 'node:fs'
import path from 'node:path'

// V2 structural plugin definition; no V1 hook adapter or package installation.
export default {
  id: 'viewer-eval-policy',
  async setup(ctx) {
    const file = process.env.OTEL_EVAL_V2_POLICY_LOG
    if (!file) throw new Error('Missing private policy log')
    const append = value => fs.appendFileSync(file, JSON.stringify({ observedAt: Date.now(), ...value }) + '\n', { mode: 0o600 })
    const sharedRequestRecord = (sessionID, value) => {
      const registry = process.env.OTEL_EVAL_V2_SESSION_REGISTRY
      if (!registry) return
      if (!/^ses_[a-zA-Z0-9]+$/.test(sessionID)) throw Error('Invalid request session identity')
      const { workspace } = JSON.parse(fs.readFileSync(path.join(registry, sessionID + '.json'), 'utf8'))
      fs.appendFileSync(path.join(path.dirname(workspace), 'requests.jsonl'), JSON.stringify({ observedAt: Date.now(), sessionID, ...value }) + '\n', { mode: 0o600 })
    }
    await ctx.skill.transform(editor => { for (const skill of editor.list()) editor.remove(skill.id) })
    await ctx.session.hook('retry', event => {
      append({ type: 'retry.veto', sessionID: event.sessionID, attempt: event.attempt, error: event.error, proposed: event.decision })
      event.decision = { retry: false }
      sharedRequestRecord(event.sessionID, { type: 'retry.veto', attempt: event.attempt })
    })
    await ctx.session.hook('title', event => { event.result = 'Viewer checkout-failure evaluation' })
    await ctx.session.hook('context', event => {
      sharedRequestRecord(event.sessionID, { type: 'model.context' })
      append({ type: 'model.context', sessionID: event.sessionID, model: event.model,
        system: event.system, messages: event.messages, tools: Object.keys(event.tools) })
    })
    await ctx.session.hook('http.response', async event => {
      if (event.response.ok) return
      append({ type: 'http.error', sessionID: event.sessionID, kind: event.kind,
        status: event.response.status, body: await event.response.clone().text() })
    })
    append({ type: 'ready', version: ctx.app.version, directory: ctx.location.directory })
    return () => append({ type: 'closed' })
  },
}
