let stop
export function onFailure(callback) { stop = callback }
export function afterEach({ result }) {
  if (result.response?.metadata?.caseOutcome === 'investigator-timeout' &&
      result.response.error === 'No answer: investigator reached its time limit' &&
      !result.response.metadata.errors?.length &&
      !result.gradingResult?.componentResults?.some(c => c.metadata?.graderError)) return
  if (result.failureReason === 2 || result.response?.error || result.gradingResult?.componentResults?.some(c => c.metadata?.graderError))
    stop?.(new Error('Stopped on execution error; original result retained'))
}
