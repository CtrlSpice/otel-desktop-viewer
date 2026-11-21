import fs from 'node:fs'
import path from 'node:path'

let stop
export function onFailure(callback) { stop = callback }
export function checkEvidenceError(error, root) {
  if (['ENOSPC', 'EDQUOT', 'EIO', 'EROFS'].includes(error?.code) ||
      (error?.path?.startsWith(root + '/') && error.syscall))
    stop?.(new Error('Stopped because run evidence is unavailable', { cause: error }))
}
export function afterEach({ result }) {
  const metadata = result.response?.metadata
  if (metadata?.harnessFailure) return stop?.(new Error('Stopped on shared investigator harness failure'))
  if (!metadata?.evidence ||
      !fs.existsSync(path.join(metadata.evidence, 'response.json')))
    stop?.(new Error('Stopped because investigator evidence could not be saved'))
}
