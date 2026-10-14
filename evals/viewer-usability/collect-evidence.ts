import fs from 'node:fs'
import path from 'node:path'
import { investigatorEvidence, ownedPath, sha256 } from './evidence.ts'
import { gradingPrompt, readViewerSkill } from './grade.ts'
import { record, saveJson, TEMPORARY } from './runtime.ts'

export async function collectEvidence(root: string, parent = TEMPORARY) {
  const pilotFile = ownedPath(root, path.join(root, 'pilot.json'))
  const pilotText = fs.readFileSync(pilotFile, 'utf8')
  const rows = record(record(JSON.parse(pilotText)).results).results
  if (!Array.isArray(rows)) throw new Error('Missing saved attempts')
  const viewerSkill = await readViewerSkill(root)
  const output = fs.mkdtempSync(path.join(parent, 'otel-grader-evidence-'))
  fs.chmodSync(output, 0o700)
  const seen = new Set<string>()
  const counts = new Map<string, number>()
  const inventory = rows.map((value, rowIndex) => {
    const row = record(value)
    const response = record(row.response)
    const taskID = record(row.vars).task_id
    if (typeof taskID !== 'string' || typeof response.output !== 'string')
      throw new Error('Missing saved answer or task')
    const packet = investigatorEvidence(
      response.metadata,
      taskID,
      response.output,
      root
    )
    if (record(row.provider).label !== packet.investigator.model)
      throw new Error('Saved row model mismatch')
    if (seen.has(packet.investigator.evidence))
      throw new Error('Duplicate saved attempt')
    seen.add(packet.investigator.evidence)
    const key = JSON.stringify([packet.investigator.model, taskID])
    const attempt = (counts.get(key) ?? 0) + 1
    counts.set(key, attempt)
    const name = path.basename(packet.investigator.evidence) + '.json'
    saveJson(path.join(output, name), packet)
    const packetText = fs.readFileSync(path.join(output, name), 'utf8')
    const prompt = gradingPrompt(
      taskID,
      response.output,
      viewerSkill,
      undefined,
      packet
    )
    const truncated = packet.events
      .filter(event => {
        const state = record(record(record(JSON.parse(event.raw)).part).state)
        return record(state.metadata ?? {}).truncated === true
      })
      .map(event => event.line)
    return {
      rowIndex,
      attempt,
      taskID,
      model: packet.investigator.model,
      originalTestIndex: row.testIdx,
      originalPromptIndex: row.promptIdx,
      investigator: packet.investigator,
      packet: name,
      packetSHA256: sha256(packetText),
      answerSHA256: packet.answerSHA256,
      sources: packet.sources,
      packetBytes: Buffer.byteLength(packetText),
      gradingPromptBytes: Buffer.byteLength(prompt),
      permissionDenials: packet.permissionDenials,
      recordedTruncatedToolLines: truncated,
      toolFailures: packet.events
        .filter(event => event.failures.length)
        .map(event => ({ line: event.line, failures: event.failures })),
    }
  })
  // Re-read only the exact allowlisted files to verify the immutable source hashes.
  for (const entry of inventory)
    for (const [name, hash] of Object.entries(entry.sources))
      if (
        sha256(
          fs.readFileSync(
            ownedPath(
              entry.investigator.evidence,
              path.join(entry.investigator.evidence, name)
            )
          )
        ) !== hash
      )
        throw new Error('Original source changed during collection')
  if (sha256(fs.readFileSync(pilotFile)) !== sha256(pilotText))
    throw new Error('Original pilot changed during collection')
  saveJson(path.join(output, 'inventory.json'), {
    sourceRoot: fs.realpathSync(root),
    pilotSHA256: sha256(pilotText),
    sourceHashesVerified: true,
    attempts: inventory,
  })
  return {
    output,
    attempts: inventory.length,
    packetBytes: [
      Math.min(...inventory.map(row => row.packetBytes)),
      Math.max(...inventory.map(row => row.packetBytes)),
    ],
    gradingPromptBytes: [
      Math.min(...inventory.map(row => row.gradingPromptBytes)),
      Math.max(...inventory.map(row => row.gradingPromptBytes)),
    ],
    permissionDeniedAttempts: inventory.filter(
      row => row.permissionDenials.length
    ).length,
    permissionDenials: inventory.reduce(
      (total, row) => total + row.permissionDenials.length,
      0
    ),
    recordedTruncatedAttempts: inventory.filter(
      row => row.recordedTruncatedToolLines.length
    ).length,
    largest: [...inventory]
      .sort((a, b) => b.gradingPromptBytes - a.gradingPromptBytes)
      .slice(0, 5)
      .map(row => ({ packet: row.packet, bytes: row.gradingPromptBytes })),
  }
}

if (
  process.argv[1] &&
  fs.realpathSync(process.argv[1]) === import.meta.filename
)
  console.log(JSON.stringify(await collectEvidence(process.argv[2]), null, 2))
