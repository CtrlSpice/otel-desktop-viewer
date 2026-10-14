import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import tasks from './tasks.json' with { type: 'json' }
import rubrics from './rubrics.json' with { type: 'json' }
import { SUITE } from './runtime.ts'

test('exact approved questions contain no private grading values or answer format', () => {
  assert.deepEqual(
    tasks.map(task => task.goal),
    [
      'Why did checkout-017 fail?',
      'Why was checkout-019 slower than checkout-018?',
      'Did the checkout failure rate change during this period? What changed alongside it?',
      'Show me all the logs for checkout-017. Can each one be tied to a span we have?',
      'How many different customers appear in the checkout traffic during this period?',
      'Are order sequence numbers stored with the same data type across checkouts?',
      'Export checkout-017 so another engineer can load it into their viewer.',
      'How did request latency change during this period?',
      'How many requests does the request counter report during this period?',
      'What CPU readings are available around checkout-019?',
    ]
  )
  assert.deepEqual(
    Object.keys(rubrics),
    tasks.map(task => task.id)
  )
  const prompt = fs.readFileSync(path.join(SUITE, 'prompt.txt'), 'utf8')
  for (const task of tasks) {
    assert.deepEqual(Object.keys(task), ['id', 'goal'])
    const rendered = prompt
      .replace('{{goal}}', task.goal)
      .replace('{{binary}}', '/synthetic/viewer')
      .replace('{{endpoint}}', 'http://127.0.0.1:1')
      .replace('{{start}}', '2026-10-07T07:54:35Z')
      .replace('{{end}}', '2026-10-07T08:04:35Z')
    assert.ok(rendered.startsWith(task.goal + '\n'))
    assert.doesNotMatch(
      rendered,
      /JSON|evidence field|900719925474|insufficient_funds|104|reported-cause|0\/8|Observability guidance|document-processing|720 ms/
    )
  }
})
