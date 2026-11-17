import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { readViewerSkill } from './grade.ts'
import { viewerSkillFixture, VIEWER_SKILL } from './test-fixture.ts'
import { TEMPORARY } from './runtime.ts'

test('app skill comes from the evaluated binary and failures remain setup errors', async () => {
  const root = fs.mkdtempSync(path.join(TEMPORARY, 'eval-viewer-skill-test-'))
  const skill = VIEWER_SKILL.replace(
    'SYNTHETIC_EVALUATED_VIEWER_GUIDE', 'OLDER_BUILD_GUIDE'
  )
  const binary = viewerSkillFixture(root, skill)
  const connection = path.join(root, 'connection.json')
  fs.writeFileSync(connection, JSON.stringify({ binary }))
  assert.equal(await readViewerSkill(root), skill)
  fs.writeFileSync(connection, JSON.stringify({ binary: 'relative-viewer' }))
  await assert.rejects(readViewerSkill(root), /evaluated viewer binary/)
  fs.writeFileSync(
    connection, JSON.stringify({ binary: path.join(root, 'missing') })
  )
  await assert.rejects(readViewerSkill(root), /ENOENT/)
  fs.writeFileSync(connection, JSON.stringify({ binary }))
  viewerSkillFixture(root, 'help output instead of the app skill')
  await assert.rejects(readViewerSkill(root), /did not return its app skill/)
  fs.writeFileSync(binary, `#!${process.execPath}\nprocess.exit(2);\n`, {
    mode: 0o700,
  })
  await assert.rejects(readViewerSkill(root), /Command failed/)
})
