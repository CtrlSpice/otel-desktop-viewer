import fs from 'node:fs'
import path from 'node:path'

export const VIEWER_SKILL = `---
name: otel-desktop-viewer
description: Synthetic evaluated viewer guide.
---

SYNTHETIC_EVALUATED_VIEWER_GUIDE
Read query results as recorded telemetry, preserving exact numeric values.
`

export function viewerSkillFixture(root: string, skill = VIEWER_SKILL) {
  const binary = path.join(root, 'viewer-skill-stub')
  fs.writeFileSync(
    binary,
    `#!${process.execPath}
if (process.argv.length !== 3 || process.argv[2] !== 'skills') process.exit(2);
process.stdout.write(${JSON.stringify(skill)});
`,
    { mode: 0o700 }
  )
  return binary
}
