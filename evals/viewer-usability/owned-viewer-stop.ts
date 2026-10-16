import path from 'node:path'
import type { ChildProcess } from 'node:child_process'
import { saveJson } from './runtime.ts'

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
      viewer.process.kill('SIGTERM')
      const timer = setTimeout(() => viewer.process.kill('SIGKILL'), 30000)
      try {
        const exit = await viewer.closed
        saveJson(path.join(root, 'owned-shutdown.json'), {
          pid: viewer.pid,
          waited: true,
          exit,
          requestedAt,
          completedAt: new Date().toISOString(),
        })
      } finally {
        clearTimeout(timer)
      }
    })())
}
