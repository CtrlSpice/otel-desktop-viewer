import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { readIsolationCheckSettings } from './isolation-check-settings.ts'
import { TEMPORARY } from './runtime.ts'

const fixture = () => {
  const root = fs.mkdtempSync(path.join(TEMPORARY, 'eval-check-settings-'))
  const file = path.join(root, 'settings.json')
  const settings = {
    authFile: path.join(root, 'auth.json'),
    personalConfig: path.join(root, 'config'),
    originalSuite: path.join(root, 'preserved'),
    opencodeBinary: path.join(root, 'opencode'),
    viewerBinary: path.join(root, 'viewer'),
    forbiddenMarkers: ['private-instruction-canary'],
  }
  fs.writeFileSync(file, JSON.stringify(settings), { mode: 0o600 })
  return { file, settings }
}

test('isolation check settings retain explicit private paths and markers', () => {
  const { file, settings } = fixture()
  assert.deepEqual(readIsolationCheckSettings(file), settings)
})

test('isolation check settings reject implicit, public and relative inputs', () => {
  assert.throws(() => readIsolationCheckSettings(''), /private absolute/)
  assert.throws(
    () => readIsolationCheckSettings('settings.json'),
    /private absolute/
  )
  const { file, settings } = fixture()
  fs.chmodSync(file, 0o644)
  assert.throws(() => readIsolationCheckSettings(file), /private absolute/)
  fs.chmodSync(file, 0o600)
  for (const key of [
    'authFile',
    'personalConfig',
    'originalSuite',
    'opencodeBinary',
    'viewerBinary',
  ]) {
    fs.writeFileSync(file, JSON.stringify({ ...settings, [key]: 'relative' }))
    assert.throws(
      () => readIsolationCheckSettings(file),
      /must be an absolute path/
    )
  }
  fs.writeFileSync(
    file,
    JSON.stringify({ ...settings, forbiddenMarkers: [42] })
  )
  assert.throws(() => readIsolationCheckSettings(file), /nonempty strings/)
})

test('malformed settings errors do not disclose input contents', () => {
  const { file } = fixture()
  fs.writeFileSync(file, '{"sensitive":"synthetic-secret" INVALID')
  assert.throws(() => readIsolationCheckSettings(file), {
    message: 'Cannot parse isolation check settings',
  })
})
