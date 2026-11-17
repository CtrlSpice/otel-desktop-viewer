export function observerEnvironment(env = process.env) {
  const endpoint = env.OTEL_EVAL_OBSERVER_HTTP
  if (!endpoint) return {}
  const url = new URL(endpoint)
  if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password || url.search || url.hash)
    throw new Error('Observer must be an HTTP(S) base URL without credentials, query or fragment')
  return {
    OTEL_EVAL_OBSERVER_HTTP: url.href.replace(/\/$/, ''),
    PROMPTFOO_OTEL_ENABLED: 'true',
    PROMPTFOO_OTEL_ENDPOINT: url.href.replace(/\/$/, '') + '/v1/traces',
    PROMPTFOO_OTEL_SERVICE_NAME: 'viewer-eval',
    PROMPTFOO_OTEL_LOCAL_EXPORT: 'false',
  }
}
