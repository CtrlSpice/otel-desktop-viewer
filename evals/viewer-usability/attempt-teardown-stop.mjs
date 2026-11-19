let stop
export function onFailure(callback) { stop = callback }

export function beforeAll({ suite }) {
  if (stop) return
  const openCode = suite.providers.some(provider => provider.id().startsWith('opencode-v2:'))
  throw new Error(openCode
    ? 'Use the OpenCode evaluation launcher so cleanup failures stop the batch.'
    : 'Use npm run evaluate --prefix evals/viewer-usability so cleanup failures stop the batch.')
}

export function afterEach({ result }) {
  const errors = [result.error, result.response?.error,
    ...(result.gradingResult?.componentResults ?? []).map(component => component.reason)]
  if (errors.some(error => typeof error === 'string' && error.includes('Harness binary teardown failed')))
    stop?.(new Error('Stopped on harness teardown failure; original evidence retained'))
}
