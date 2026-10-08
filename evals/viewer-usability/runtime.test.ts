import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { runtimeRoot, SUITE, TEMPORARY } from './runtime.ts'

test('runtime must be explicit, absolute and outside repositories', () => {
  const previous = process.env.OTEL_EVAL_RUN_DIR
  try {
    delete process.env.OTEL_EVAL_RUN_DIR
    assert.throws(runtimeRoot, /Set OTEL_EVAL_RUN_DIR/)
    process.env.OTEL_EVAL_RUN_DIR = '.'
    assert.throws(runtimeRoot, /absolute/)
    process.env.OTEL_EVAL_RUN_DIR = SUITE
    assert.throws(runtimeRoot, /outside repositories/)
    const directory = fs.mkdtempSync(
      path.join(TEMPORARY, 'otel-eval-path-check-')
    )
    process.env.OTEL_EVAL_RUN_DIR = directory
    assert.equal(runtimeRoot(), fs.realpathSync(directory))
    const missing = path.join(directory, 'missing')
    process.env.OTEL_EVAL_RUN_DIR = missing
    assert.throws(runtimeRoot, /ENOENT/)
    const file = path.join(directory, 'file')
    fs.writeFileSync(file, '')
    process.env.OTEL_EVAL_RUN_DIR = file
    assert.throws(runtimeRoot, /directory/)
    const link = path.join(directory, 'repo-link')
    fs.symlinkSync(SUITE, link)
    process.env.OTEL_EVAL_RUN_DIR = link
    assert.throws(runtimeRoot, /outside repositories/)
    process.env.OTEL_EVAL_RUN_DIR = directory
    fs.writeFileSync(path.join(directory, '.git'), 'gitdir: placeholder\n', {
      flag: 'wx',
    })
    assert.throws(runtimeRoot, /outside repositories/)
    // Preserve test artifacts rather than removing directories implicitly.
  } finally {
    if (previous === undefined) delete process.env.OTEL_EVAL_RUN_DIR
    else process.env.OTEL_EVAL_RUN_DIR = previous
  }
})
