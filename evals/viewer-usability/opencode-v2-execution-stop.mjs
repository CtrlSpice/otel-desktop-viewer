import fs from 'node:fs'
import path from 'node:path'

let stop
export function onFailure(callback) { stop = callback }
export function isEvidenceError(error, root) {
  return ['ENOSPC', 'EDQUOT', 'EIO', 'EROFS'].includes(error?.code) ||
    Boolean(error?.path?.startsWith(root + '/') && error.syscall)
}
export function stopRun(error) { stop?.(error) }
export function checkEvidenceError(error, root) {
  if (isEvidenceError(error, root)) stopRun(error)
}
export function afterEach({ result }) {
  const metadata = result.response?.metadata
  if (metadata?.harnessFailure) return stopRun(new Error(result.response.error))
  if (!metadata?.evidence ||
      !fs.existsSync(path.join(metadata.evidence, 'response.json')))
    stop?.(new Error('Stopped because investigator evidence could not be saved'))
}
