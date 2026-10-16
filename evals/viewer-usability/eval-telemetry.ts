import fs from 'node:fs'
import path from 'node:path'
import http from 'node:http'
import { once } from 'node:events'
import { randomBytes } from 'node:crypto'
import { AsyncLocalStorage } from 'node:async_hooks'
import { context, propagation, trace, ROOT_CONTEXT, SpanStatusCode, isSpanContextValid } from '@opentelemetry/api'
import type { Context, Span, SpanContext } from '@opentelemetry/api'
import { record } from './runtime.ts'

// Promptfoo's externally-owned OpenCode client has no per-call header option.
// Scope injection to the exact owned origin and async call; never send context
// to model providers, the observer exporter, or another concurrent attempt.
const requests = new AsyncLocalStorage<{ origin: string; parent: Context }>()
let installed = false
export function withOpenCodeContext<T>(baseURL: string, parent: Context, run: () => Promise<T>) {
  if (!installed) {
    const original = globalThis.fetch
    globalThis.fetch = async (input, init) => {
      const active = requests.getStore()
      const url = input instanceof Request ? input.url : String(input)
      if (!active || new URL(url).origin !== active.origin) return original(input, init)
      const headers = new Headers(init?.headers ?? (input instanceof Request ? input.headers : undefined))
      const carrier: Record<string, string> = {}
      propagation.inject(active.parent, carrier)
      for (const [key, value] of Object.entries(carrier)) headers.set(key, value)
      return original(input, { ...init, headers })
    }
    installed = true
  }
  return requests.run({ origin: new URL(baseURL).origin, parent }, run)
}

const tracer = () => trace.getTracer('viewer-eval', '1')
type Entry = { span: Span; context: Context; ended: boolean }
type AttemptOptions = {
  root: string; evidence: string; model: string; role: string; task: string
  parent?: string; redact: (text: string) => string
}
const id = (value: unknown) => {
  if (typeof value !== 'string' || !value) throw new Error('Telemetry event identity missing')
  return value
}
const error = (span: Span, message: string) => {
  span.recordException({ name: 'EvaluationError', message })
  span.setStatus({ code: SpanStatusCode.ERROR, message })
}

export class AttemptTelemetry {
  readonly options: AttemptOptions
  readonly span: Span
  readonly context: Context
  readonly token = randomBytes(32).toString('hex')
  readonly turns = new Map<string, Entry>()
  readonly tools = new Map<string, Entry>()
  readonly server: http.Server
  endpoint = ''
  ready = false
  drained = false
  failure?: string
  private finished = false
  private turnNumber = 0

  constructor(options: AttemptOptions) {
    this.options = options
    const parent = options.parent
      ? propagation.extract(ROOT_CONTEXT, { traceparent: options.parent }) : context.active()
    this.span = tracer().startSpan(`${options.role} ${options.task}`, {
      attributes: {
        'eval.run.id': path.basename(options.root), 'eval.attempt.id': path.basename(options.evidence),
        'eval.task.id': options.task, 'eval.role': options.role, 'gen_ai.request.model': options.model,
      },
    }, parent)
    if (!isSpanContextValid(this.span.spanContext())) throw new Error('Promptfoo tracing must be initialised before the attempt')
    this.context = trace.setSpan(parent, this.span)
    this.server = http.createServer(async (req, res) => {
      if (req.method !== 'POST' || req.url !== '/hook' || req.headers.authorization !== 'Bearer ' + this.token) {
        res.writeHead(403).end(); return
      }
      try {
        let size = 0
        const chunks: Buffer[] = []
        for await (const chunk of req) {
          size += chunk.length
          if (size > 16 * 1024 * 1024) throw new Error('Telemetry hook body too large')
          chunks.push(chunk)
        }
        const raw = options.redact(Buffer.concat(chunks).toString())
        const event = record(JSON.parse(raw))
        fs.appendFileSync(path.join(options.evidence, 'telemetry-events.jsonl'), raw + '\n', { mode: 0o600 })
        const response = this.accept(event)
        res.writeHead(200, { 'Content-Type': 'application/json' }).end(JSON.stringify(response))
      } catch (cause) {
        this.failure = options.redact(cause instanceof Error ? cause.message : 'Telemetry hook failed')
        res.writeHead(500).end('Telemetry hook failed')
      }
    })
  }

  async listen() {
    this.server.listen(0, '127.0.0.1')
    await once(this.server, 'listening')
    const address = this.server.address()
    if (!address || typeof address === 'string') throw new Error('Missing telemetry hook address')
    this.endpoint = `http://127.0.0.1:${address.port}/hook`
    return this
  }

  private link(span: Span, supplied: unknown) {
    if (!supplied) return
    const value = record(supplied)
    const sc = { traceId: value.traceId, spanId: value.spanId, traceFlags: value.traceFlags } as SpanContext
    if (isSpanContextValid(sc) && Number.isInteger(sc.traceFlags)) {
      span.addLink({ context: sc, attributes: { 'eval.link.kind': 'native-opencode-detail' } })
    }
  }

  private tool(sessionID: string, callID: string, messageID: string | undefined, name: string, native?: unknown) {
    const key = sessionID + '/' + callID
    let entry = this.tools.get(key)
    if (!entry) {
      const parent = messageID ? this.turns.get(sessionID + '/' + messageID)?.context ?? this.context : this.context
      const span = tracer().startSpan('tool ' + name, { attributes: {
        'session.id': sessionID, 'gen_ai.tool.call.id': callID, 'gen_ai.tool.name': name,
        ...(messageID ? { 'gen_ai.message.id': messageID } : {}),
      } }, parent)
      entry = { span, context: trace.setSpan(parent, span), ended: false }
      this.tools.set(key, entry)
    }
    if (!entry.ended) this.link(entry.span, native)
    return entry
  }

  accept(event: Record<string, unknown>): Record<string, string> {
    if (event.type === 'ready') { this.ready = true; this.link(this.span, event.native); return {} }
    if (event.type === 'event') {
      const data = record(event.event)
      const props = record(data.properties ?? {})
      if (data.type === 'message.updated') {
        const info = record(props.info)
        if (info.role !== 'assistant') return {}
        const messageID = id(info.id), sessionID = id(info.sessionID)
        const key = sessionID + '/' + messageID
        let entry = this.turns.get(key)
        const time = record(info.time)
        if (!entry) {
          const span = tracer().startSpan(`agent.turn ${++this.turnNumber}`, {
            startTime: typeof time.created === 'number' ? new Date(time.created) : undefined,
            attributes: { 'session.id': sessionID, 'gen_ai.message.id': messageID,
              'gen_ai.request.model': String(info.modelID ?? this.options.model) },
          }, this.context)
          entry = { span, context: trace.setSpan(this.context, span), ended: false }
          this.turns.set(key, entry)
          this.link(span, event.native)
        }
        if (!entry.ended) {
          entry.span.setAttribute('opencode.message', JSON.stringify(info))
          if (info.tokens) {
            const tokens = record(info.tokens)
            for (const key of ['input', 'output', 'reasoning'])
              if (typeof tokens[key] === 'number') entry.span.setAttribute('gen_ai.usage.' + key + '_tokens', tokens[key])
          }
          if (typeof info.finish === 'string') entry.span.setAttribute('gen_ai.response.finish_reason', info.finish)
          if (info.error) error(entry.span, JSON.stringify(info.error))
          if (typeof time.completed === 'number') {
            entry.span.end(new Date(time.completed)); entry.ended = true
          }
        }
      } else if (data.type === 'message.part.updated') {
        const part = record(props.part)
        if (part.type === 'text' || part.type === 'reasoning') {
          const entry = this.turns.get(String(part.sessionID) + '/' + String(part.messageID))
          if (entry && !entry.ended && typeof part.text === 'string')
            entry.span.setAttribute('gen_ai.output.' + part.type, part.text)
          return {}
        }
        if (part.type !== 'tool') return {}
        const state = record(part.state)
        if (!['running', 'completed', 'error'].includes(String(state.status))) return {}
        const entry = this.tool(id(part.sessionID), id(part.callID), id(part.messageID), id(part.tool), event.native)
        if (!entry.ended) {
          entry.span.setAttribute('gen_ai.tool.call.arguments', JSON.stringify(state.input ?? {}))
          if (['completed', 'error'].includes(String(state.status))) this.endTool(entry, state)
        }
      }
      return {}
    }
    if (event.type === 'tool.before' || event.type === 'tool.after' || event.type === 'shell.env') {
      const entry = this.tool(id(event.sessionID), id(event.callID),
        typeof event.messageID === 'string' ? event.messageID : undefined,
        typeof event.tool === 'string' ? event.tool : 'shell', event.native)
      if (event.type === 'tool.before' && !entry.ended) entry.span.setAttribute('gen_ai.tool.call.arguments', JSON.stringify(event.args ?? {}))
      if (event.type === 'tool.after' && !entry.ended) this.endTool(entry, record(event.result))
      if (event.type === 'shell.env') {
        if (entry.ended) throw new Error('Cannot propagate an already completed tool')
        const carrier: Record<string, string> = {}
        propagation.inject(entry.context, carrier)
        return carrier
      }
      return {}
    }
    if (event.type === 'drain') { this.drained = true; return {} }
    throw new Error('Unknown telemetry hook event')
  }

  private endTool(entry: Entry, result: Record<string, unknown>) {
    const metadata = record(result.metadata ?? {})
    if (result.output !== undefined) entry.span.setAttribute('gen_ai.tool.call.result', String(result.output))
    if (typeof metadata.exit === 'number') entry.span.setAttribute('process.exit.code', metadata.exit)
    if (result.status === 'error' || result.error || (typeof metadata.exit === 'number' && metadata.exit !== 0))
      error(entry.span, String(result.error ?? 'Tool process exited with code ' + metadata.exit))
    entry.span.end(); entry.ended = true
  }

  identity() {
    const sc = this.span.spanContext()
    return { traceID: sc.traceId, spanID: sc.spanId }
  }

  async finish(outcome?: { error?: string; output?: unknown }) {
    if (this.finished) return
    this.finished = true
    if (this.server.listening)
      await new Promise<void>((resolve, reject) => this.server.close(err => err ? reject(err) : resolve()))
    const incomplete = [...this.turns.values(), ...this.tools.values()].filter(entry => !entry.ended)
    for (const entry of incomplete) {
      error(entry.span, outcome?.error ?? 'Native operation did not report completion')
      entry.span.setAttribute('eval.interrupted', true)
      entry.span.end(); entry.ended = true
    }
    if (outcome?.output !== undefined) this.span.setAttribute('gen_ai.output', this.options.redact(String(outcome.output)))
    this.failure ??= !this.ready ? 'Native telemetry plugin did not initialise'
      : !this.drained ? 'Native telemetry plugin did not drain'
      : incomplete.length ? 'Native operations did not report completion' : undefined
    const failure = outcome?.error ?? this.failure
    if (failure) error(this.span, this.options.redact(failure))
    this.span.setAttribute('eval.incomplete_operations', incomplete.length)
    this.span.end()
    fs.writeFileSync(path.join(this.options.evidence, 'telemetry.json'), JSON.stringify({
      ...this.identity(), ready: this.ready, drained: this.drained, failure: failure ?? null,
      turns: this.turns.size, tools: this.tools.size, incompleteOperations: incomplete.length,
      nativeDetail: 'Native OpenCode spans are linked; pinned 1.18.10 does not propagate HTTP context into session internals.',
    }, null, 2), { mode: 0o600, flag: 'wx' })
  }
}
