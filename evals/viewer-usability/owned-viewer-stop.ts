import path from 'node:path'
import type { ChildProcess } from 'node:child_process'
import { saveJson } from './runtime.ts'
import { stopOwnedGroup } from './run-cancellation.ts'

export function ownedViewerStop(
  root: string,
  viewer: {
    process: ChildProcess
    pid?: number
    closed: Promise<{ code: number | null; signal: string | null }>
  }
) {
  let stopped: Promise<void> | undefined
  return () =>
    (stopped ??= (async () => {
      const requestedAt = new Date().toISOString()
      if (viewer.process.pid) await stopOwnedGroup(viewer.process.pid, viewer.process)
      const exit = await viewer.closed
      saveJson(path.join(root, 'owned-shutdown.json'), {
        pid: viewer.pid,
        waited: true,
        exit,
        requestedAt,
        completedAt: new Date().toISOString(),
      })
    })())
}
