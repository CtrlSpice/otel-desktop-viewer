import test from 'node:test'
import assert from 'node:assert/strict'
import { telemetryLinks } from './telemetry-links.ts'
import { aggregate } from './aggregate.ts'
import { render } from './render.ts'

const traceID = 'a'.repeat(32), spanID = 'b'.repeat(16)

test('task runs and grader outputs render clickable trace and tool-error links', () => {
  const investigator = { model: 'test/investigator', telemetry: { traceID, spanID, tools: [{ spanID: 'c'.repeat(16), name: 'bash', error: true }] } }
  const grade = { role: 'grader', taskID: 'checkout-latency', status: 'pass', score: 1, investigator, metadata: { model: 'test/grader', telemetry: { traceID, spanID: 'd'.repeat(16) } } }
  const data = aggregate({ models: [investigator.model], tasks: 1, repetitions: 1, telemetryViewer: '' }, { results: { results: [{ provider: { label: investigator.model }, vars: { task_id: grade.taskID }, success: true, response: { metadata: investigator }, gradingResult: { score: 1, componentResults: [{ metadata: grade }] } }] } }, {}, { judgments: [grade], models: [{ model: 'test/grader', attempts: 1 }] })
  const html = render(data)
  assert.ok(html.includes(`/traces/${traceID}?span=${spanID}`))
  assert.ok(html.includes(`/traces/${traceID}?span=${'c'.repeat(16)}`))
  assert.ok(html.includes(`/traces/${traceID}?span=${'d'.repeat(16)}`))
  assert.match(html, /class="tool-error">Error/)
  assert.match(html, /grader trace/)
})

test('telemetry links preserve exact span identities and error calls without private payloads', () => {
  const links = telemetryLinks({ traceID, spanID, tools: [{ spanID: 'c'.repeat(16), name: 'bash', error: true, command: '/private/secret' }] }, { telemetryViewer: 'http://127.0.0.1:8000' })!
  assert.equal(links.span, `http://127.0.0.1:8000/traces/${traceID}?span=${spanID}`)
  assert.deepEqual(links.tools, [{ traceID, spanID: 'c'.repeat(16), href: `http://127.0.0.1:8000/traces/${traceID}?span=${'c'.repeat(16)}`, name: 'bash', error: true, number: 1, startedAt: null }])
  assert.doesNotMatch(JSON.stringify(links), /private|secret|command/)
  assert.equal(telemetryLinks({ traceID, spanID }, { telemetryViewer: '' })?.span, `/traces/${traceID}?span=${spanID}`)
})

test('recorded observers override the default; unsafe origins and malformed identities do not become links', () => {
  assert.equal(telemetryLinks({ traceID, spanID }, { telemetryViewer: 'http://localhost:8000', telemetryViewers: { [traceID]: 'http://localhost:9000' } })?.trace, `http://localhost:9000/traces/${traceID}`)
  for (const telemetryViewer of ['javascript:alert(1)', 'file:///private/run', 'http://user:secret@localhost:8000', 'http://localhost:8000/private', 'http://localhost:8000/?token=secret']) assert.equal(telemetryLinks({ traceID, spanID }, { telemetryViewer }), null)
  for (const invalid of ['0'.repeat(32), '../secret', '<script>', undefined]) assert.equal(telemetryLinks({ traceID: invalid, spanID }, { telemetryViewer: '' }), null)
  assert.equal(telemetryLinks({ traceID, spanID: '../secret' }, { telemetryViewer: '' })?.span, null)
  assert.equal(telemetryLinks({ traceID, spanID }, {}), null)
})

test('tool timestamps match span identities and invalid or absent times remain unknown', () => {
  const a = { spanID: 'c'.repeat(16), name: 'bash', error: false }
  const b = { spanID: 'd'.repeat(16), name: 'bash', error: true }
  const time = '2026-10-16T18:21:32.646Z'
  const manifest = { telemetryViewer: '', telemetryTools: { [traceID]: [{ ...b, startedAt: time }, a] } }
  const links = telemetryLinks({ traceID, tools: [a, b] }, manifest)!
  assert.deepEqual(links.tools.map(tool => tool.startedAt), [null, time])
  for (const startedAt of ['secret', '2026-02-30T18:21:32.646Z', '2026-10-16', 123, undefined]) {
    assert.equal(telemetryLinks({ traceID, tools: [{ ...a, startedAt }] }, manifest)!.tools[0].startedAt, null)
  }
  assert.equal(telemetryLinks({ traceID, tools: [{ ...a, startedAt: time }] }, manifest)!.tools[0].startedAt, time)
})
