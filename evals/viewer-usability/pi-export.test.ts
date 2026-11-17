import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { exportWorkspace } from './verify-export.ts'
import { TEMPORARY } from './runtime.ts'

for (const runtime of ['pi', 'claude-code']) test(`${runtime} export verification accepts only its investigator-owned workspace`, () => {
  const root = fs.mkdtempSync(path.join(TEMPORARY, 'pi-export-'))
  const evidence = path.join(root, 'runs/investigator-one')
  const workspace = path.join(evidence, 'workspace')
  fs.mkdirSync(workspace, { recursive: true })
  const metadata = { runtime, role: 'investigator', evidence, workspace }
  assert.equal(exportWorkspace(metadata, root), fs.realpathSync(workspace))
  assert.throws(() => exportWorkspace({ ...metadata, role: 'grader' }, root))
  const sibling = path.join(root, 'runs/investigator-two/workspace')
  fs.mkdirSync(sibling, { recursive: true })
  assert.throws(() => exportWorkspace({ ...metadata, workspace: sibling }, root))
  const link = path.join(evidence, 'linked-workspace')
  fs.symlinkSync(sibling, link)
  assert.throws(() => exportWorkspace({ ...metadata, workspace: link }, root))
})
