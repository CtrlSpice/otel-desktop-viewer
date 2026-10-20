import type { ChildProcess } from 'node:child_process'
import { setTimeout as delay } from 'node:timers/promises'

// Keep handlers installed until asynchronous owned-process cleanup has finished.
export function runCancellation(parent?: AbortSignal) {
  const controller = new AbortController()
  const interrupt = () => controller.abort(new Error('Evaluation cancelled by SIGINT'))
  const terminate = () => controller.abort(new Error('Evaluation cancelled by SIGTERM'))
  const inherit = () => controller.abort(parent?.reason)
  process.on('SIGINT', interrupt)
  process.on('SIGTERM', terminate)
  parent?.addEventListener('abort', inherit, { once: true })
  if (parent?.aborted) inherit()
  return {
    signal: controller.signal,
    cancel: (reason: Error) => controller.abort(reason),
    dispose() {
      process.off('SIGINT', interrupt)
      process.off('SIGTERM', terminate)
      parent?.removeEventListener('abort', inherit)
    },
  }
}

// Call only for a group created by this run (or reported by its IPC worker).
// No saved PID files, process-name searches, or port-based process discovery.
export async function stopOwnedGroup(pid: number, child?: ChildProcess, graceMs = 30000) {
  const target = process.platform === 'win32' ? pid : -pid
  const signal = (value: NodeJS.Signals | 0) => {
    try { process.kill(target, value); return true }
    catch (error) {
      if (error instanceof Error && 'code' in error && error.code === 'ESRCH') return false
      // EPERM from a liveness probe still means the group exists (including
      // the short macOS exit/reaping window). Never interpret it as cleanup.
      if (value === 0 && error instanceof Error && 'code' in error && error.code === 'EPERM') return true
      throw error
    }
  }
  const closed = child && child.exitCode === null && child.signalCode === null
    ? new Promise<void>(resolve => child.once('close', () => resolve()))
    : Promise.resolve()
  signal('SIGTERM')
  const deadline = performance.now() + graceMs
  while (signal(0)) {
    if (performance.now() >= deadline) {
      signal('SIGKILL')
      break
    }
    await delay(50)
  }
  await closed
  // Await group disappearance, including descendants that ignored SIGTERM.
  const killedDeadline = performance.now() + 5000
  while (signal(0)) {
    if (performance.now() >= killedDeadline) throw new Error('Owned process group did not exit: ' + pid)
    await delay(50)
  }
}
