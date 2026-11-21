import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { createHash } from 'node:crypto'
import { saveJson } from './runtime.ts'

type BinaryFixture = {
  directory: string
  canonicalBinary?: string
  kind: 'attempt' | 'export-replay'
}

async function fingerprint(file: string) {
  const handle = await fs.promises.open(file, fs.constants.O_RDONLY | fs.constants.O_NOFOLLOW)
  try {
    const before = await handle.stat({ bigint: true })
    assert.ok(before.isFile(), 'Executable must be a regular file')
    const hash = createHash('sha256')
    for await (const chunk of handle.createReadStream({ autoClose: false })) hash.update(chunk)
    const after = await handle.stat({ bigint: true })
    assert.equal(after.ino, before.ino, 'Executable changed during verification')
    assert.equal(after.size, before.size, 'Executable changed during verification')
    assert.equal(after.mtimeNs, before.mtimeNs, 'Executable changed during verification')
    return { sha256: hash.digest('hex'), device: before.dev, inode: before.ino,
      size: before.size, mtimeNs: before.mtimeNs }
  } finally {
    await handle.close()
  }
}

function unchanged(file: string, expected: Awaited<ReturnType<typeof fingerprint>>) {
  const actual = fs.lstatSync(file, { bigint: true })
  assert.ok(actual.isFile() && !actual.isSymbolicLink(), 'Executable was replaced')
  assert.equal(actual.dev, expected.device, 'Executable was replaced')
  assert.equal(actual.ino, expected.inode, 'Executable was replaced')
  assert.equal(actual.size, expected.size, 'Executable changed after verification')
  assert.equal(actual.mtimeNs, expected.mtimeNs, 'Executable changed after verification')
}

async function teardownBinary(fixture: BinaryFixture, attemptError: unknown) {
  assert.ok(path.isAbsolute(fixture.directory), 'Teardown requires an absolute owned directory')
  assert.ok(!fs.lstatSync(fixture.directory).isSymbolicLink(), 'Teardown directory must not be a symlink')
  const directory = fs.realpathSync(fixture.directory)
  assert.ok(['attempt', 'export-replay'].includes(fixture.kind), 'Unknown binary fixture kind')
  const relative = fixture.kind === 'attempt' ? 'workspace/bin/otel-desktop-viewer' : 'otel-desktop-viewer'
  const executable = path.join(directory, relative)
  const receipt = path.join(directory, 'teardown.json')
  const identity = { executable, ...(attemptError === undefined ? {} : { attemptError: String(attemptError) }) }
  try {
    let current = directory
    for (const component of relative.split('/')) {
      current = path.join(current, component)
      let stat
      try { stat = fs.lstatSync(current) }
      catch (error) {
        if (error instanceof Error && 'code' in error && error.code === 'ENOENT') {
          saveJson(receipt, { ...identity, status: 'not-present' })
          return
        }
        throw error
      }
      assert.ok(!stat.isSymbolicLink(), 'Teardown refuses executable paths containing symlinks')
    }
    assert.ok(fixture.canonicalBinary && path.isAbsolute(fixture.canonicalBinary), 'Retained canonical binary is required')
    const canonical = fs.realpathSync(fixture.canonicalBinary)
    assert.notEqual(canonical, executable, 'Never remove the canonical binary')
    const retained = await fingerprint(canonical)
    const copy = await fingerprint(executable)
    assert.equal(copy.sha256, retained.sha256, 'Changed executable is evidence and must be retained')
    unchanged(canonical, retained)
    unchanged(executable, copy)
    // Claim the deletion before unlinking. A repeated or unrecordable cleanup
    // must fail rather than silently remove a file.
    saveJson(path.join(directory, 'teardown-started.json'), {
      ...identity, canonicalBinary: canonical, sha256: retained.sha256, fileBytes: copy.size.toString(),
    })
    unchanged(canonical, retained)
    unchanged(executable, copy)
    fs.unlinkSync(executable)
    saveJson(receipt, { ...identity, status: 'removed', canonicalBinary: canonical,
      sha256: retained.sha256, removedFileBytes: copy.size.toString() })
  } catch (error) {
    if (!fs.existsSync(receipt)) saveJson(receipt, { ...identity, status: 'failed', error: String(error) })
    throw error
  }
}

type Shutdown = () => Promise<void>
export type RegisterShutdown = (shutdown: Shutdown) => void

/** Save evidence in execute. Shutdown must finish before the executable is removed. */
export async function withBinaryTeardown<T>(fixture: BinaryFixture, execute: (register: RegisterShutdown) => Promise<T>): Promise<T> {
  let attemptError: unknown
  let shutdown: Shutdown = async () => {}
  try {
    return await execute(action => { shutdown = action })
  } catch (error) {
    attemptError = error
    throw error
  } finally {
    try {
      await shutdown()
      await teardownBinary(fixture, attemptError)
    } catch (cleanupError) {
      const errors = attemptError === undefined ? [cleanupError] : [attemptError, cleanupError]
      const receipt = path.join(fixture.directory, 'teardown.json')
      try {
        if (!fs.existsSync(receipt)) saveJson(receipt, { status: 'failed', error: String(cleanupError),
          ...(attemptError === undefined ? {} : { attemptError: String(attemptError) }) })
      } catch (receiptError) { errors.push(receiptError) }
      const failure = new AggregateError(errors,
        `Harness binary teardown failed; see ${path.join(fixture.directory, 'teardown.json')}`)
      const hook = './attempt-teardown-stop.mjs'
      const { teardownFailed } = await import(hook)
      teardownFailed(failure)
      throw failure
    }
  }
}
