import fs from 'node:fs'
import path from 'node:path'
import { createHash } from 'node:crypto'
import base from './opencode-v2-policy.mjs'
import { workspacePath, workspaceSearch } from './opencode-v2-workspace-guard.mjs'
import { checkCurlCommand } from './opencode-v2-curl.mjs'

export default {
  id: 'viewer-eval-policy',
  async setup(ctx) {
    const cleanup = await base.setup(ctx)
    const directory = process.env.OTEL_EVAL_V2_SESSION_REGISTRY
    if (!directory) throw new Error('Missing owned evaluation session registry')
    const registration = sessionID => {
      if (!/^ses_[a-zA-Z0-9]+$/.test(sessionID)) throw new Error('Invalid evaluation session identity')
      const value = JSON.parse(fs.readFileSync(path.join(directory, sessionID + '.json'), 'utf8'))
      if (value.sessionID !== sessionID) throw new Error('Evaluation session registry identity mismatch')
      return value
    }
    const hooks = []
    hooks.push(await ctx.permission.hook('evaluate', event => {
      if (!['read', 'edit'].includes(event.action)) return
      try {
        const owned = registration(event.sessionID)
        for (const resource of event.resources) workspacePath(owned.workspace, resource, event.action === 'edit')
      } catch (error) { event.effect = 'deny'; event.message = error.message }
    }))
    hooks.push(await ctx.tool.hook('execute.before', event => {
      const owned = registration(event.sessionID)
      if (['glob', 'grep'].includes(event.tool)) workspaceSearch(owned.workspace, event.input.path ?? '.')
      if (event.tool === 'shell') {
        checkCurlCommand(event.input.command, owned.fixtureEndpoint)
        const binary = path.join(owned.workspace, 'bin/otel-desktop-viewer')
        if (fs.lstatSync(binary).isSymbolicLink() || createHash('sha256').update(fs.readFileSync(binary)).digest('hex') !== owned.binarySHA256)
          throw new Error('The evaluated viewer executable changed')
      }
    }))
    hooks.push(await ctx.shell.hook('create.before', event => {
      const cwd = fs.realpathSync(event.cwd)
      const owned = fs.readdirSync(directory).filter(file => file.endsWith('.json')).map(file => JSON.parse(fs.readFileSync(path.join(directory, file), 'utf8')))
        .find(value => value.workspace === cwd)
      if (!owned) throw new Error('Shell working directory is not an owned evaluation workspace')
      const fixture = new URL(owned.fixtureEndpoint)
      if (fixture.protocol !== 'http:' || fixture.hostname !== '127.0.0.1' || !fixture.port || fixture.origin !== owned.fixtureEndpoint)
        throw new Error('Expected the assigned loopback fixture origin')
      event.shell = path.join(path.dirname(directory), 'policy', 'fixture-shell')
      const q = JSON.stringify
      const helpers = [...['curl', 'ls', 'head', 'jq', 'grep', 'wc'].map(name => path.join(process.env.OTEL_EVAL_CURL_BIN, name)),
        ...['opencode-v2-curl.mjs', 'opencode-v2-workspace-guard.mjs', 'opencode-v2-stdin-filter.mjs'].map(name => path.join(path.dirname(event.shell), name))]
      const profile = ['(version 1)', '(allow default)',
        ...JSON.parse(process.env.OTEL_EVAL_SHELL_PROTECTED).map(file => `(deny file-read-data (subpath ${q(file)}))`),
        `(allow file-read-data (subpath ${q(cwd)}) ${helpers.map(file => `(literal ${q(file)})`).join(' ')})`,
        '(deny file-write*)',
        `(allow file-write* (subpath ${q(cwd)}) (literal "/dev/null"))`,
        `(deny file-write* (subpath ${q(path.join(cwd, 'bin'))}))`,
        '(deny network-outbound)', `(allow network-outbound (remote ip "localhost:${fixture.port}"))`,
      ].join('\n')
      const temporary = path.join(cwd, '.tmp')
      fs.mkdirSync(temporary, { recursive: true, mode: 0o700 })
      event.env = { PATH: `${process.env.OTEL_EVAL_CURL_BIN}:/opt/homebrew/bin:/usr/bin:/bin`, HOME: cwd, TMPDIR: temporary, LANG: 'en_US.UTF-8',
        OTEL_EVAL_CURL_ENDPOINT: owned.fixtureEndpoint,
        OTEL_EVAL_SHELL_PROFILE: profile }
    }))
    return async () => { await Promise.all(hooks.map(hook => hook.dispose())); cleanup?.() }
  },
}
