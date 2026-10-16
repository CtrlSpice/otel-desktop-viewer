import path from 'node:path'

const denialPrefix =
  'The user has specified a rule which prevents you from using this specific tool call. Here are some of the relevant rules '
const clients = new Set([
  'help',
  'skills',
  'services',
  'attributes',
  'query',
  'traces',
  'trace',
  'span',
  'logs',
  'log',
  'metrics',
  'metric',
  'export',
  'import',
])
const valueFlags = new Set([
  '--endpoint',
  '--limit',
  '--service',
  '--since',
  '--start',
  '--end',
  '--signal',
  '--owner-type',
  '--series',
  '--host',
  '--http',
  '--grpc',
  '--browser-port',
  '--db',
  '--db-max-size',
  '--self-telemetry-endpoint',
])

export function viewerClientCall(command: string, workdir: string, viewers: string[]) {
  // Keep quoted arguments together so echoing a command is not an invocation.
  const tokens =
    command.match(
      /\d*[<>]+(?:&[\d-]+)?|(?:"(?:\\.|[^"\\])*"|'[^']*'|\\.|[^\s;&|()<>])+|[;&|()\n]/g
    ) ?? []
  let words: string[] = []
  let directory = workdir
  const client = () => {
    if (words[0] === 'cd' && words.length === 2 && !/[$`]/.test(words[1])) {
      directory = path.resolve(directory, words[1])
      return false
    }
    let index = 0
    while (/^[A-Za-z_][A-Za-z0-9_]*=/.test(words[index] ?? '')) index++
    if (words[index] === 'env' || words[index] === '/usr/bin/env') {
      index++
      while (/^[A-Za-z_][A-Za-z0-9_]*=/.test(words[index] ?? '')) index++
    }
    if (words[index] === 'command' || words[index] === 'exec') index++
    const executable = words[index]
    if (!executable) return false
    const supplied =
      executable === 'otel-desktop-viewer' ||
      viewers.includes(path.resolve(directory, executable))
    if (!supplied) return false
    // Cobra accepts value flags before subcommands. Do not match their values.
    while (++index < words.length) {
      const argument = words[index]
      if (!argument.startsWith('-')) return clients.has(argument)
      if (argument === '--') return false
      const equals = argument.indexOf('=')
      const flag = equals < 0 ? argument : argument.slice(0, equals)
      const value = equals < 0 ? undefined : argument.slice(equals + 1)
      if (flag === '--help' || flag === '-h' || flag === '--version') {
        if (value === undefined || /^(?:1|t|T|true|TRUE|True)$/.test(value))
          return true
        if (/^(?:0|f|F|false|FALSE|False)$/.test(value)) continue
        return false
      }
      if (valueFlags.has(flag)) {
        if (value === undefined && ++index >= words.length) return false
      } else if (
        flag === '--open-browser' ||
        (flag === '--json' && value !== undefined)
      ) {
        // --json is local to clients: before a subcommand it needs =value.
        if (
          value !== undefined &&
          !/^(?:1|0|t|T|f|F|true|TRUE|True|false|FALSE|False)$/.test(value)
        )
          return false
      } else return false
    }
    return false
  }
  let redirect = false
  for (const token of tokens) {
    if (/^\d*[<>]+(?:&[\d-]+)?$/.test(token)) {
      // Redirection targets are not command arguments (including leading redirects).
      redirect = !token.includes('&')
      continue
    }
    if (redirect) { redirect = false; continue }
    if (/^[;&|()\n]$/.test(token)) {
      if (client()) return true
      words = []
    } else {
      words.push(
        token.replace(
          /'([^']*)'|"((?:\\.|[^"\\])*)"|\\(.)/g,
          (_, single, double, escaped) =>
            single ?? double?.replace(/\\(["\\$`])/g, '$1') ?? escaped
        )
      )
    }
  }
  return client()
}

export function permissionDenial(
  line: string,
  workspace: string,
  viewers: string[]
) {
  let event
  try {
    event = JSON.parse(line)
  } catch {
    return
  }
  const part = event?.part
  const state = part?.state
  if (
    event?.type !== 'tool_use' ||
    part?.type !== 'tool' ||
    typeof event.sessionID !== 'string' ||
    typeof part.callID !== 'string' ||
    typeof part.tool !== 'string' ||
    state?.status !== 'error' ||
    typeof state.error !== 'string' ||
    !state.error.startsWith(denialPrefix)
  )
    return
  let rules
  try {
    rules = JSON.parse(state.error.slice(denialPrefix.length))
  } catch {
    return
  }
  if (
    !Array.isArray(rules) ||
    !rules.some(
      rule =>
        rule?.action === 'deny' &&
        typeof rule.permission === 'string' &&
        typeof rule.pattern === 'string'
    )
  )
    return
  const input = state.input
  const stopsRun =
    part.tool === 'bash' &&
    typeof input?.command === 'string' &&
    viewerClientCall(
      input.command,
      typeof input.workdir === 'string' ? input.workdir : workspace,
      viewers
    )
  return {
    sessionID: event.sessionID,
    callID: part.callID,
    tool: part.tool,
    input,
    reason: state.error,
    stopsRun,
    rawEvent: line,
  }
}
