import fs from 'node:fs'
import path from 'node:path'
import os from 'node:os'
import { readJson, record, SUITE } from './runtime.ts'

export const SUPPORTED_OPENCODE_VERSION = '1.18.10'
const NETWORK_KEYS = [
  'HTTP_PROXY',
  'HTTPS_PROXY',
  'ALL_PROXY',
  'NO_PROXY',
  'http_proxy',
  'https_proxy',
  'all_proxy',
  'no_proxy',
  'SSL_CERT_FILE',
  'SSL_CERT_DIR',
  'NODE_EXTRA_CA_CERTS',
]

type EnvironmentOptions = {
  root: string
  inherited?: NodeJS.ProcessEnv
  executablePaths?: string[]
  authEnvironment?: Record<string, string>
}

export function credentialEnvironment(value: unknown) {
  const result: Record<string, string> = {}
  for (const [key, entry] of Object.entries(record(value))) {
    if (typeof entry !== 'string')
      throw new Error(
        'Only explicit provider credential environment keys are accepted'
      )
    result[key] = entry
  }
  return result
}

export function cleanEnvironment({
  root,
  inherited = process.env,
  executablePaths = [],
  authEnvironment = {},
}: EnvironmentOptions) {
  const env: Record<string, string> = {}
  for (const key of NETWORK_KEYS) if (inherited[key]) env[key] = inherited[key]
  for (const [key, value] of Object.entries(authEnvironment)) {
    if (
      !/^[A-Z][A-Z0-9_]*(?:API_KEY|TOKEN)$/.test(key) ||
      key.startsWith('OPENCODE_')
    ) {
      throw new Error(
        'Only explicit provider credential environment keys are accepted'
      )
    }
    env[key] = value
  }
  return Object.assign(env, {
    HOME: path.join(root, 'home'),
    XDG_CONFIG_HOME: path.join(root, 'config'),
    XDG_DATA_HOME: path.join(root, 'data'),
    XDG_STATE_HOME: path.join(root, 'state'),
    XDG_CACHE_HOME: path.join(root, 'cache'),
    TMPDIR: path.join(root, 'tmp'),
    PATH: [
      ...executablePaths,
      '/opt/homebrew/bin',
      '/usr/bin',
      '/bin',
      '/usr/sbin',
      '/sbin',
    ].join(path.delimiter),
    SHELL: '/bin/sh',
    LANG: 'en_US.UTF-8',
    OPENCODE_TEST_HOME: path.join(root, 'home'),
    OPENCODE_CONFIG_DIR: path.join(root, 'config', 'opencode'),
    OPENCODE_TEST_MANAGED_CONFIG_DIR: path.join(root, 'managed'),
    OPENCODE_DISABLE_PROJECT_CONFIG: '1',
    OPENCODE_DISABLE_CLAUDE_CODE: '1',
    OPENCODE_DISABLE_EXTERNAL_SKILLS: '1',
    OPENCODE_PURE: '1',
    OPENCODE_DISABLE_AUTOUPDATE: '1',
    OPENCODE_DISABLE_LSP_DOWNLOAD: '1',
  })
}

export type CleanOptions = {
  parent?: string
  model?: string
  authFile?: string
  providerIDs?: string[]
  permission?: unknown
  executablePaths?: string[]
  inherited?: NodeJS.ProcessEnv
  authEnvironment?: Record<string, string>
}

export function createCleanEnvironment({
  parent = SUITE,
  model,
  authFile,
  providerIDs = [],
  permission,
  executablePaths = [],
  inherited = process.env,
  authEnvironment = {},
}: CleanOptions) {
  if (!model || !/^[^/]+\/.+$/.test(model))
    throw new Error('An explicit provider/model ID is required')
  if (
    !permission ||
    typeof permission !== 'object' ||
    Array.isArray(permission) ||
    !Object.keys(permission).length
  ) {
    throw new Error('Caller must supply the approved permission rules')
  }
  const resolvedParent = fs.realpathSync(parent)
  for (let current = resolvedParent; ; current = path.dirname(current)) {
    if (fs.existsSync(path.join(current, '.git')))
      throw new Error('Clean scratch must be outside repositories')
    if (path.dirname(current) === current) break
  }
  if (process.platform === 'darwin') {
    const user = os.userInfo().username
    if (
      [
        path.join(
          '/Library/Managed Preferences',
          user,
          'ai.opencode.managed.plist'
        ),
        '/Library/Managed Preferences/ai.opencode.managed.plist',
      ].some(file => fs.existsSync(file))
    ) {
      throw new Error(
        'Managed macOS preferences prevent clean isolation in this CLI version'
      )
    }
  }
  for (const directory of executablePaths) {
    if (!path.isAbsolute(directory))
      throw new Error('Executable search directories must be absolute')
  }
  const root = fs.mkdtempSync(path.join(resolvedParent, 'clean-context-'))
  fs.chmodSync(root, 0o700)
  const env = cleanEnvironment({
    root,
    inherited,
    executablePaths,
    authEnvironment,
  })
  const workspace = path.join(root, 'scratch')
  const directories = [
    workspace,
    env.HOME,
    env.XDG_CONFIG_HOME,
    env.XDG_DATA_HOME,
    env.XDG_STATE_HOME,
    env.XDG_CACHE_HOME,
    env.TMPDIR,
    env.OPENCODE_CONFIG_DIR,
    env.OPENCODE_TEST_MANAGED_CONFIG_DIR,
    path.join(env.XDG_DATA_HOME, 'opencode'),
    path.join(env.OPENCODE_CONFIG_DIR, 'node_modules'),
  ]
  for (const directory of directories)
    fs.mkdirSync(directory, { recursive: true, mode: 0o700 })
  const write = (file: string, value: unknown) =>
    fs.writeFileSync(file, JSON.stringify(value, null, 2), {
      flag: 'wx',
      mode: 0o600,
    })
  if (authFile) {
    const source = record(readJson(authFile))
    const selected: Record<string, unknown> = {}
    for (const id of new Set([model.split('/')[0], ...providerIDs])) {
      if (!source[id]) continue
      const entry = record(source[id])
      if (entry.type !== 'oauth' && entry.type !== 'api') {
        throw new Error(
          'Remote-config authentication is not safe for clean context'
        )
      }
      selected[id] = source[id]
    }
    write(path.join(env.XDG_DATA_HOME, 'opencode', 'auth.json'), selected)
  }
  const config = {
    $schema: 'https://opencode.ai/config.json',
    model,
    permission,
    autoupdate: false,
    share: 'disabled',
  }
  write(path.join(env.OPENCODE_CONFIG_DIR, 'opencode.json'), config)
  // Version-specific skip condition prevents installing the built-in plugin dependency.
  write(path.join(env.OPENCODE_CONFIG_DIR, 'package.json'), { private: true })
  write(path.join(env.OPENCODE_CONFIG_DIR, 'package-lock.json'), {
    lockfileVersion: 3,
    packages: {
      '': {
        dependencies: { '@opencode-ai/plugin': SUPPORTED_OPENCODE_VERSION },
      },
    },
  })
  return { root, workspace, env, config }
}
