import { context, trace } from '@opentelemetry/api'

// Trusted, isolated 1.18.10 hook adapter. No prompts, tools or permissions are
// changed. Ordered hook delivery preserves actual message and tool identities.
export default async function () {
  const endpoint = process.env.OTEL_EVAL_HOOK_ENDPOINT
  const token = process.env.OTEL_EVAL_HOOK_TOKEN
  if (!endpoint || !token) throw new Error('Missing eval telemetry hook configuration')
  let pending = Promise.resolve()
  let failure
  const messages = new Map()
  const messageID = input => messages.get(input.sessionID + '/' + input.callID)
  const native = () => {
    const sc = trace.getSpan(context.active())?.spanContext()
    return sc ? { traceId: sc.traceId, spanId: sc.spanId, traceFlags: sc.traceFlags } : undefined
  }
  const send = data => {
    const captured = { ...data, native: native() }
    const next = pending.then(async () => {
      if (failure) throw failure
      const response = await fetch(endpoint, {
        method: 'POST', headers: { 'Content-Type': 'application/json', Authorization: 'Bearer ' + token },
        body: JSON.stringify(captured), signal: AbortSignal.timeout(10000),
      })
      if (!response.ok) throw new Error('Eval telemetry hook failed')
      return response.json()
    })
    // Event callbacks are not awaited by OpenCode. Observe failures here and
    // surface them at the next awaited hook or dispose instead of losing them.
    pending = next.then(() => {}, error => { failure = error })
    return next
  }
  await send({ type: 'ready' })
  return {
    event: ({ event }) => {
      if (!['message.updated', 'message.part.updated'].includes(event.type)) return
      const part = event.properties?.part
      if (part?.type === 'tool') messages.set(part.sessionID + '/' + part.callID, part.messageID)
      void send({ type: 'event', event }).catch(() => {})
    },
    'tool.execute.before': async (input, output) => {
      await send({ type: 'tool.before', ...input, messageID: messageID(input), args: output.args })
    },
    'tool.execute.after': async (input, output) => {
      await send({ type: 'tool.after', ...input, messageID: messageID(input), result: output })
    },
    'shell.env': async (input, output) => {
      if (!input.sessionID || !input.callID) return
      const carrier = await send({ type: 'shell.env', ...input, messageID: messageID(input) })
      output.env.TRACEPARENT = carrier.traceparent
      output.env.TRACESTATE = carrier.tracestate ?? ''
      output.env.OTEL_EVAL_HOOK_TOKEN = ''
      output.env.OTEL_EVAL_HOOK_ENDPOINT = ''
    },
    dispose: async () => { await send({ type: 'drain' }); if (failure) throw failure },
  }
}
