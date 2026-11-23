import { spawnSync } from 'node:child_process'

// The shell passes the command name separately from the investigator's argv.
// Only a jq filter or a grep pattern is an operand; input always comes from stdin.
const [name, ...args] = process.argv.slice(2)
const flags = { jq: /^-[rcse]+$/, grep: /^-[EFiinvoc]+$/, wc: /^-[clmw]+$/ }
try {
  if (!Object.hasOwn(flags, name)) throw Error('Unknown stdin filter')
  const options = []
  while (args.length && flags[name].test(args[0])) options.push(args.shift())
  if (args[0] === '--') args.shift()
  if (name === 'wc' && args.length || name === 'grep' && args.length !== 1 || name === 'jq' && args.length > 1)
    throw Error(`${name} accepts stdin only; file operands are not supported`)
  if (name === 'jq') {
    const filter = args[0] ?? '.'
    // Plain quoted keys and values are data. Interpolation can contain jq code.
    const code = filter.includes('\\(') ? filter : filter.replace(/"(?:[^"\\\r\n]|\\[^\r\n])*"/g, '""')
    if (filter.startsWith('-') || /\b(?:import|include|_*modulemeta)\b/.test(code))
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
