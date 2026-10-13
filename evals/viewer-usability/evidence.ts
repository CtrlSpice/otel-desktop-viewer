import fs from 'node:fs'
import path from 'node:path'
import { createHash } from 'node:crypto'
import { record, runtimeRoot } from './runtime.ts'

export const sha256 = (text: string | Buffer) =>
  createHash('sha256').update(text).digest('hex')

// Canonicalise only the runtime prefix (macOS /var aliases /private/var).
// Every component below it must be owned, real and not a symlink.
export function ownedPath(root: string, supplied: unknown) {
  if (typeof supplied !== 'string' || !path.isAbsolute(supplied))
    throw new Error('Evidence requires an absolute owned path')
  const canonicalRoot = fs.realpathSync(root)
  let ancestor = path.resolve(supplied)
  const suffix: string[] = []
  while (fs.realpathSync(ancestor) !== canonicalRoot) {
    if (fs.lstatSync(ancestor).isSymbolicLink())
      throw new Error('Evidence symlink rejected')
    suffix.unshift(path.basename(ancestor))
    const parent = path.dirname(ancestor)
    if (parent === ancestor) throw new Error('Evidence path outside runtime')
    ancestor = parent
  }
  return path.join(canonicalRoot, ...suffix)
}

export function toolEvidence(stdout: string, sessions: string[]) {
  const events: { line: number; raw: string; failures: string[] }[] = []
  for (const [index, raw] of stdout.split('\n').entries()) {
    if (!raw.trim()) continue
    const event = record(JSON.parse(raw))
    if (typeof event.type !== 'string')
      throw new Error('Malformed evidence event')
    if (
      event.sessionID !== undefined &&
      !sessions.includes(String(event.sessionID))
    )
      throw new Error('Cross-session evidence rejected')
    if (event.type !== 'tool_use') continue
    const part = record(event.part)
    const state = record(part.state)
    if (
      typeof part.tool !== 'string' ||
      !['completed', 'error'].includes(String(state.status))
    )
      throw new Error('Missing or incomplete tool evidence')
    record(state.input)
    for (const id of [part.sessionID])
      if (id !== undefined && !sessions.includes(String(id)))
        throw new Error('Cross-session tool evidence rejected')
    if (state.status === 'completed' && typeof state.output !== 'string')
      throw new Error('Missing tool output')
    if (state.status === 'error' && typeof state.error !== 'string')
      throw new Error('Missing tool error')
    const metadata = record(state.metadata ?? {})
    const failures: string[] = []
    if (state.status === 'error') failures.push('tool-error')
    if (typeof metadata.exit === 'number' && metadata.exit !== 0)
      failures.push('command-exit')
    if (
      state.status === 'error' &&
      typeof state.error === 'string' &&
      state.error.startsWith(
        'The user has specified a rule which prevents you from using this specific tool call.'
      )
    )
      failures.push('native-permission-denial-needs-harness-review')
    // Only parsed response envelopes count, not quoted error prose in successful data.
    if (typeof state.output === 'string') {
      for (const candidate of [state.output, ...state.output.split('\n')]) {
        try {
          const response = record(JSON.parse(candidate))
          if (response.jsonrpc === '2.0' && response.error !== undefined) {
            failures.push('json-rpc-error')
            break
          }
        } catch {}
      }
    }
    events.push({ line: index + 1, raw, failures })
  }
  return events
}

export function investigatorEvidence(
  value: unknown,
  taskID: string,
  answer: string,
  root = runtimeRoot()
) {
  try {
    const metadata = record(value)
    if (
      metadata.role !== 'investigator' ||
      metadata.task !== taskID ||
      typeof metadata.model !== 'string'
    )
      throw new Error('Attempt identity mismatch')
    const evidence = ownedPath(root, metadata.evidence)
    if (
      path.dirname(evidence) !== path.join(fs.realpathSync(root), 'runs') ||
      !path.basename(evidence).startsWith('investigator-')
    )
      throw new Error('Not an investigator evidence directory')
    const isolation = ownedPath(root, metadata.isolationRoot)
    const scratch = ownedPath(root, metadata.workspace)
    if (
      path.dirname(isolation) !==
        path.join(fs.realpathSync(root), 'workspaces') ||
      !path.basename(isolation).startsWith('clean-context-') ||
      scratch !== path.join(isolation, 'scratch')
    )
      throw new Error('Attempt scratch ownership mismatch')
    const sources: Record<string, string> = {}
    const read = (name: string) => {
      const file = ownedPath(evidence, path.join(evidence, name))
      if (!fs.statSync(file).isFile()) throw new Error('Evidence is not a file')
      const text = fs.readFileSync(file, 'utf8')
      sources[name] = sha256(text)
      return text
    }
    const metrics = record(JSON.parse(read('metrics.json')))
    for (const key of [
      'role',
      'task',
      'model',
      'workspace',
      'isolationRoot',
      'evidence',
    ])
      if (metrics[key] !== metadata[key])
        throw new Error('Attempt metadata mismatch')
    if (
      !Array.isArray(metrics.sessionIDs) ||
      !metrics.sessionIDs.every(id => typeof id === 'string') ||
      JSON.stringify(metrics.sessionIDs) !== JSON.stringify(metadata.sessionIDs)
    )
      throw new Error('Attempt session mismatch')
    if (read('answer.txt') !== answer)
      throw new Error('Attempt answer mismatch')
    const events = toolEvidence(
      read('stdout.jsonl'),
      metrics.sessionIDs as string[]
    )
    if (metrics.toolCalls !== events.length)
      throw new Error('Tool evidence count mismatch')
    // Hash these intended source artifacts, but never deliver investigator prompts.
    read('prompt.txt')
    if (!Array.isArray(JSON.parse(read('commands.json'))))
      throw new Error('Malformed commands evidence')
    return {
      investigator: {
        taskID,
        model: metrics.model,
        evidence,
        scratch,
        sessionIDs: metrics.sessionIDs,
      },
      answerSHA256: sha256(answer),
      sources,
      sessionExit: metrics.exit,
      events,
      permissionDenials: events
        .filter(event =>
          event.failures.includes(
            'native-permission-denial-needs-harness-review'
          )
        )
        .map(event => event.line),
    }
  } catch (error) {
    throw new Error(
      'Investigator evidence harness issue: ' +
        (error instanceof Error ? error.message : 'invalid evidence')
    )
  }
}
