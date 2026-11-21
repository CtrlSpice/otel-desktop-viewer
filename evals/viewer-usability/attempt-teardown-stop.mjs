let stop
export function onFailure(callback) { stop = callback }

export function beforeAll({ suite }) {
  if (stop) return
  const openCode = suite.providers.some(provider => provider.id().startsWith('opencode-v2:'))
  throw new Error(openCode
    ? 'Use the OpenCode evaluation launcher so cleanup failures stop the batch.'
    : 'Use npm run evaluate --prefix evals/viewer-usability so cleanup failures stop the batch.')
}

export function teardownFailed(error) { stop?.(error) }
