import assert from 'node:assert/strict'
import test from 'node:test'
import { observerEnvironment } from './clean-environment.ts'

test('observer setup requires an explicit endpoint and rejects embedded credentials', () => {
  assert.deepEqual(observerEnvironment({}), {})
  assert.throws(() => observerEnvironment({ OTEL_EVAL_OBSERVER_HTTP: 'http://user:secret@localhost:4318' }))
  assert.equal(observerEnvironment({ OTEL_EVAL_OBSERVER_HTTP: 'http://localhost:4318/' }).PROMPTFOO_OTEL_ENDPOINT, 'http://localhost:4318/v1/traces')
})

test('observer configuration supplies the native exporter settings without changing the input', () => {
  const env = { OTEL_EVAL_OBSERVER_HTTP: 'https://observer.example/otlp/', UNRELATED: 'retained' }
  assert.deepEqual(observerEnvironment(env), {
    OTEL_EVAL_OBSERVER_HTTP: 'https://observer.example/otlp',
    PROMPTFOO_OTEL_ENABLED: 'true',
    PROMPTFOO_OTEL_ENDPOINT: 'https://observer.example/otlp/v1/traces',
    PROMPTFOO_OTEL_SERVICE_NAME: 'viewer-eval',
    PROMPTFOO_OTEL_LOCAL_EXPORT: 'false',
  })
  assert.deepEqual(env, { OTEL_EVAL_OBSERVER_HTTP: 'https://observer.example/otlp/', UNRELATED: 'retained' })
  for (const endpoint of ['file:///tmp/observer', 'http://localhost:4318?token=private', 'http://localhost:4318/#fragment'])
    assert.throws(() => observerEnvironment({ OTEL_EVAL_OBSERVER_HTTP: endpoint }), /HTTP\(S\) base URL/)
})
