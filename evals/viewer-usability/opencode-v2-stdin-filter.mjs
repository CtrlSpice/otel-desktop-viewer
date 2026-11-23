import { spawnSync } from 'node:child_process'

// The shell passes the command name separately from the investigator's argv.
// Only a jq filter or a grep pattern is an operand; input always comes from stdin.
const [name, ...args] = process.argv.slice(2)
const flags = { jq: /^-[rcse]+$/, grep: /^-[EFiinvoc]+$/, wc: /^-[clmw]+$/ }
// Examine identifiers in code, including interpolation expressions. Keep
// nested strings and parentheses separate so a quoted key is always data.
function usesModule(filter) {
  const modes = ['code']
  for (let i = 0; i < filter.length; i++) {
    const ch = filter[i]
    if (modes.at(-1) === 'string') {
      if (ch === '"') modes.pop()
      else if (ch === '\\' && filter[++i] === '(') modes.push('expression')
    } else if (ch === '"') modes.push('string')
    else if (ch === '#') {
      const end = filter.slice(i).search(/[\r\n]/)
      if (end < 0) break
      i += end
    } else if (ch === '(') modes.push('expression')
    else if (ch === ')' && modes.at(-1) === 'expression') modes.pop()
    else if (/[A-Za-z_]/.test(ch)) {
      const start = i
      while (i + 1 < filter.length && /[A-Za-z0-9_]/.test(filter[i + 1])) i++
      if (filter[start - 1] !== '.' && /^(?:import|include|_*modulemeta)$/.test(filter.slice(start, i + 1))) return true
    }
  }
  return false
}
try {
  if (!Object.hasOwn(flags, name)) throw Error('Unknown stdin filter')
  const options = []
  while (args.length && flags[name].test(args[0])) options.push(args.shift())
  if (args[0] === '--') args.shift()
  if (name === 'wc' && args.length || name === 'grep' && args.length !== 1 || name === 'jq' && args.length > 1)
    throw Error(`${name} accepts stdin only; file operands are not supported`)
  if (name === 'jq') {
    const filter = args[0] ?? '.'
    if (filter.startsWith('-') || usesModule(filter))
      throw Error('Use an inline jq filter without file options or modules')
    options.push('-L', '/var/empty', '--', filter)
  }
  if (name === 'grep') options.push('-e', args[0])
  const child = spawnSync(`/usr/bin/${name}`, options, {
    stdio: 'inherit', env: { LANG: 'en_US.UTF-8', HOME: '/var/empty' },
  })
  if (child.error) throw child.error
  process.exitCode = child.status ?? 1
} catch (error) {
  console.error(error.message)
  process.exitCode = 2
}
