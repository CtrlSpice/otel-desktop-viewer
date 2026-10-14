import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { spawnSync } from 'node:child_process'
import { create } from 'tar'
import tasks from './tasks.json' with { type: 'json' }
import rubrics from './rubrics.json' with { type: 'json' }
import { investigatorEvidence, sha256 } from './evidence.ts'
import { readJson, record, saveJson, TEMPORARY } from './runtime.ts'
import {
  prepareReview, recordReview, signoffReview, showReview, validateDecision, signoffBlockers,
} from './human-review.ts'
import type { HumanDecision } from './human-review.ts'

// Entirely synthetic saved reports. No provider, network, model or viewer is used.
function fixture(taskID = 'checkout-failure', failedTool = false) {
  const parent = fs.realpathSync(fs.mkdtempSync(path.join(TEMPORARY, 'human-review-synthetic-')))
  const run = path.join(parent, 'run')
  const review = path.join(parent, 'review')
  const evidence = path.join(run, 'runs/investigator-example')
  const graderEvidence = path.join(run, 'runs/grader-example')
  const isolationRoot = path.join(run, 'workspaces/clean-context-example')
  const scratch = path.join(isolationRoot, 'scratch')
  const workspace = path.join(run, 'original-workspace')
  for (const directory of [run, evidence, graderEvidence, scratch, workspace]) fs.mkdirSync(directory, { recursive: true, mode: 0o700 })
  const answer = taskID === 'trace-export'
    ? 'Exported to handoff.json.'
    : 'The payment was declined. Inventory caused the failure. The deployment caused the decline.'
  const events = [{ type: 'tool_use', sessionID: 'ses_synthetic', part: {
    tool: 'bash', sessionID: 'ses_synthetic', state: { input: { command: 'synthetic telemetry' }, status: 'completed', output: 'Stock below threshold; reservation succeeded. Card declined: insufficient funds', metadata: { exit: 0 } },
  } }]
  if (failedTool) events.push({ type: 'tool_use', sessionID: 'ses_synthetic', part: {
    tool: 'bash', sessionID: 'ses_synthetic', state: { input: { command: 'synthetic empty search' }, status: 'completed', output: 'No matching data', metadata: { exit: 1 } },
  } })
  const investigator = {
    role: 'investigator', task: taskID, model: 'synthetic/model', evidence, isolationRoot,
    workspace, archivedWorkspace: scratch, sessionIDs: ['ses_synthetic'], toolCalls: events.length,
    exit: { code: 0, timedOut: false },
  }
  fs.writeFileSync(path.join(evidence, 'answer.txt'), answer, { mode: 0o600 })
  fs.writeFileSync(path.join(evidence, 'stdout.jsonl'), events.map(event => JSON.stringify(event)).join('\n') + '\n', { mode: 0o600 })
  fs.writeFileSync(path.join(evidence, 'prompt.txt'), 'Synthetic investigator question', { mode: 0o600 })
  saveJson(path.join(evidence, 'commands.json'), [])
  saveJson(path.join(evidence, 'metrics.json'), investigator)
  const rubric = record(rubrics)[taskID]
  const metadata = { role: 'grader', task: taskID, model: 'synthetic/grader', evidence: graderEvidence, sessionIDs: ['ses_synthetic_grader'], exit: { code: 0, timedOut: false } }
  const packet = investigatorEvidence(investigator, taskID, answer, run)
  const data = { question: tasks.find(task => task.id === taskID)!.goal, ...record(rubric), criteria: listCriteria(rubric), answer, evidence: packet }
  const prompt = () => fs.writeFileSync(path.join(graderEvidence, 'prompt.txt'), 'Synthetic grader prompt\n' + JSON.stringify(data) + '\n', { mode: 0o600 })
  prompt()
  saveJson(path.join(graderEvidence, 'metrics.json'), metadata)
  const report: Record<string, unknown> = {
    provisional: true, role: 'grader', taskID, metadata, investigator,
    investigatorEvidence: packet, status: 'pass', score: 1,
    criteria: [{ id: record(listCriteria(rubric)[0]).id, verdict: 'Pass', score: 1, explanation: 'Synthetic automated verdict, not an actual model judgment.' }],
  }
  const grading = 'runs/grader-example/grading.json'
  const writeReport = () => fs.writeFileSync(path.join(run, grading), JSON.stringify(report, null, 2) + '\n', { mode: 0o600 })
  writeReport()
  const decision = (score = 1, action: 'accept' | 'correct' = 'accept'): HumanDecision => ({
    schemaVersion: 1, reviewer: 'synthetic-human',
    completion: { action, score, reason: 'Explicit synthetic human review fixture.', evidence: [{ source: 'answer', quote: answer }] },
    claims: [], findings: [], toolReviews: [],
  })
  return { parent, run, review, grading, report, writeReport, data, prompt, answer, scratch, workspace, evidence, graderEvidence, investigator, decision }
}

function listCriteria(rubric: unknown) { return record(rubric).criteria as unknown[] }

function fingerprint(root: string): Record<string, string> {
  return Object.fromEntries(fs.readdirSync(root, { recursive: true }).map(String).filter(name => fs.statSync(path.join(root, name)).isFile()).sort().map(name => [name, sha256(fs.readFileSync(path.join(root, name)))]))
}

async function exportFixture(options: { pass?: boolean; proof?: boolean; differentPayload?: boolean; wrongMember?: boolean; archive?: boolean } = {}) {
  const f = fixture('trace-export')
  const bytes = Buffer.from('{"synthetic":"saved verified export payload"}\n')
  let member: string | undefined
  let file = 'handoff.json'
  if (options.archive) {
    file = 'handoff.tar.gz'
    const archiveRoot = path.join(f.parent, 'archive-input')
    fs.mkdirSync(archiveRoot)
    fs.writeFileSync(path.join(archiveRoot, 'trace.json'), bytes)
    fs.writeFileSync(path.join(f.scratch, file), await create({ cwd: archiveRoot, gzip: true, portable: true }, ['trace.json']).concat())
    member = 'trace.json'
    fs.writeFileSync(path.join(f.evidence, 'answer.txt'), 'Exported to handoff.tar.gz.', { mode: 0o600 })
    f.data.answer = 'Exported to handoff.tar.gz.'
    f.data.evidence = investigatorEvidence(f.investigator, 'trace-export', f.data.answer, f.run)
    f.report.investigatorEvidence = f.data.evidence
    f.prompt()
  } else fs.writeFileSync(path.join(f.scratch, file), bytes)
  f.report.artifact = { pass: options.pass ?? true, files: [path.join(f.workspace, file)], reason: 'Synthetic saved deterministic result.' }
  if (options.proof !== false) {
    const replay = path.join(f.run, 'export-replay-synthetic')
    fs.mkdirSync(path.join(replay, 'fixture'), { recursive: true })
    saveJson(path.join(replay, 'export-proof.json'), { file: path.join(f.scratch, file), pass: true, retainedSpans: 6, member: options.wrongMember ? 'wrong.json' : member })
    fs.writeFileSync(path.join(replay, 'fixture/trace.json'), options.differentPayload ? 'different bytes' : bytes)
  }
  f.writeReport()
  const decision = f.decision()
  decision.completion!.evidence = [{ source: 'answer', quote: 'Exported to ' + file + '.' }]
  decision.completion!.export = { file: path.join(f.workspace, file), location: { source: 'answer', quote: file } }
  return { ...f, file, decision }
}

test('prepare snapshots current-rubric sources privately without human decisions or run writes', async () => {
  const f = fixture()
  fs.writeFileSync(path.join(f.run, 'unrelated-private-settings.json'), '{"syntheticPrivateValue":"do-not-copy"}')
  const before = fingerprint(f.run)
  const packet = await prepareReview(f.run, f.grading, f.review)
  assert.deepEqual(fingerprint(f.run), before)
  assert.deepEqual(packet.originalAutomated, f.report)
  assert.equal(packet.investigator.model, 'synthetic/model')
  assert.deepEqual(packet.unscoredReasons, [])
  const draft = record(readJson(path.join(f.review, 'draft.json')))
  assert.equal(draft.reviewer, null)
  assert.deepEqual(draft.completion, { action: null, score: null, reason: null, evidence: null })
  assert.equal(draft.claims, null)
  assert.equal(draft.findings, null)
  assert.equal(draft.toolReviews, null)
  assert.throws(() => recordReview(f.review, draft))
  assert.equal(showReview(f.review).status, 'pending')
  assert.equal(showReview(f.review).finalScore, null)
  assert.equal(showReview(f.review).claimCounts, null)
  assert.equal(showReview(f.review).signedOff, false)
  assert.equal(fs.statSync(f.review).mode & 0o777, 0o700)
  for (const name of ['source.json', 'draft.json', 'audit.md', ...packet.sources.map(source => source.snapshot)]) assert.equal(fs.statSync(path.join(f.review, name)).mode & 0o777, 0o600)
  assert.ok(!packet.sources.some(source => source.origin.includes('unrelated-private-settings')))
  assert.match(fs.readFileSync(path.join(f.review, 'audit.md'), 'utf8'), /not automated exhaustive claim coverage/)
  await assert.rejects(prepareReview(f.run, f.grading, f.review), /exists/)
})

test('accept is pending until explicit signoff; corrections preserve history and invalidate prior final status', async () => {
  const f = fixture()
  await prepareReview(f.run, f.grading, f.review)
  const before = fingerprint(f.run)
  const pending = recordReview(f.review, f.decision())
  assert.equal(pending.status, 'pending')
  assert.equal(pending.latestRevisionSigned, false)
  assert.equal(pending.signedOff, false)
  assert.deepEqual(pending.claimCounts, { supported: 0, contradicted: 0, unverified: 0 })
  assert.equal(pending.finalScore, null)
  assert.throws(() => signoffReview(f.review, 1, 'synthetic-human', false), /confirmation/)
  assert.throws(() => signoffReview(f.review, 2, 'synthetic-human', true), /current revision/)
  assert.throws(() => signoffReview(f.review, 1, 'another-human', true), /reviewer/)
  const signed = signoffReview(f.review, 1, 'synthetic-human', true)
  assert.equal(signed.status, 'final')
  assert.equal(signed.latestRevisionSigned, true)
  assert.equal(signed.signedOff, true)
  assert.equal(signed.run, f.run)
  assert.equal(signed.taskID, 'checkout-failure')
  assert.deepEqual(signed.investigator, { evidence: f.evidence, sessionIDs: ['ses_synthetic'], model: 'synthetic/model' })
  assert.equal(signed.finalScore, 1)
  const oldEvent = fs.readFileSync(path.join(f.review, 'events/000001.json'))
  const oldSignoff = fs.readFileSync(path.join(f.review, 'events/000002.json'))
  const correction = recordReview(f.review, f.decision(0.5, 'correct'))
  assert.equal(correction.revision, 2)
  assert.equal(correction.status, 'pending')
  assert.equal(correction.latestRevisionSigned, false)
  assert.equal(correction.signedOff, false)
  assert.equal(correction.finalScore, null)
  assert.equal(correction.originalAutomated.score, 1)
  assert.equal(correction.human?.completion?.score, 0.5)
  assert.throws(() => signoffReview(f.review, 1, 'synthetic-human', true), /current revision/)
  assert.equal(signoffReview(f.review, 2, 'synthetic-human', true).finalScore, 0.5)
  assert.deepEqual(fs.readFileSync(path.join(f.review, 'events/000001.json')), oldEvent)
  assert.deepEqual(fs.readFileSync(path.join(f.review, 'events/000002.json')), oldSignoff)
  assert.deepEqual(fingerprint(f.run), before)
  assert.match(signed.identityNotice, /self-declared/)
})

test('pure score validation enforces all approved task scales and accept semantics', async () => {
  const f = fixture()
  const packet = await prepareReview(f.run, f.grading, f.review)
  for (const task of tasks) {
    const selected = { ...packet, taskID: task.id, approvedRubric: record(rubrics)[task.id] as typeof packet.approvedRubric }
    for (const score of [0, 0.5, 1]) {
      const decision = f.decision(score, 'correct')
      if (task.id === 'trace-export' && score === 1) assert.throws(() => validateDecision(decision, selected), /Export score 1/)
      else if (score === 0.5 && ['customer-count', 'trace-export', 'request-count'].includes(task.id)) assert.throws(() => validateDecision(decision, selected), /not defined/)
      else assert.equal(validateDecision(decision, selected).completion?.score, score)
    }
  }
  for (const score of [-1, 0.25, 0.75, 2, NaN]) assert.throws(() => validateDecision(f.decision(score, 'correct'), packet), /not defined/)
  // The main-owned new task uses the same rubric-defined 0/0.5/1 mechanics.
  // These descriptions are explicitly synthetic, not a replacement task rubric.
  const cpuPacket = { ...packet, taskID: 'cpu-causation', approvedRubric: { criteria: [{
    id: 'synthetic-cpu-causation', scoring: { Fail: 'Synthetic wrong answer.', Partial: 'Synthetic partial answer.', Pass: 'Synthetic complete answer.' },
  }] } }
  for (const score of [0, 0.5, 1]) assert.equal(validateDecision(f.decision(score, 'correct'), cpuPacket).completion?.score, score)
  assert.throws(() => validateDecision(f.decision(0, 'accept'), packet), /preserve/)
  const unsupported = f.decision()
  unsupported.completion!.evidence = []
  assert.throws(() => validateDecision(unsupported, packet), /Evidence/)
})

test('claims retain contradicted/unverified distinctions; unresolved decisions block signoff; extra findings do not alter score', async () => {
  const f = fixture()
  const packet = await prepareReview(f.run, f.grading, f.review)
  const decision = f.decision(0.5, 'correct')
  decision.claims = [
    { id: 'c1', claim: 'Inventory caused the failure.', claimSource: { source: 'answer', quote: 'Inventory caused the failure.' }, classification: 'contradicted', decision: 'Fail', evidence: [{ source: 'tool-event-1', quote: 'reservation succeeded' }], reason: 'Synthetic contradiction review.' },
    { id: 'c2', claim: 'The deployment caused the decline.', claimSource: { source: 'answer', quote: 'The deployment caused the decline.' }, classification: 'unverified', decision: null, evidence: [{ source: 'tool-event-1', quote: 'Card declined: insufficient funds' }], reason: 'Synthetic evidence does not prove deployment causation.' },
  ]
  decision.findings = [{ finding: 'Provider reported insufficient funds.', evidence: [{ source: 'tool-event-1', quote: 'Card declined: insufficient funds' }], reason: 'Synthetic additional finding.' }]
  assert.equal(validateDecision(decision, packet).completion?.score, 0.5)
  recordReview(f.review, decision)
  assert.throws(() => signoffReview(f.review, 1, 'synthetic-human', true), /unresolved/)
  decision.claims[1].decision = 'Pass'
  recordReview(f.review, decision)
  const final = signoffReview(f.review, 2, 'synthetic-human', true)
  assert.equal(final.human?.claims[0].classification, 'contradicted')
  assert.equal(final.human?.claims[1].classification, 'unverified')
  assert.equal(final.human?.claims[1].decision, 'Pass')
  assert.deepEqual(final.claimCounts, { supported: 0, contradicted: 1, unverified: 1 })
  assert.equal(final.finalScore, 0.5)
  assert.equal(final.human?.findings.length, 1)
  decision.claims[0].decision = 'Pass'
  assert.throws(() => recordReview(f.review, decision), /contradicted/)
  decision.claims[0].classification = 'supported'
  const amended = recordReview(f.review, decision)
  assert.equal(amended.status, 'pending')
  assert.deepEqual(amended.claimCounts, { supported: 1, contradicted: 0, unverified: 1 })
  assert.equal(amended.signedOff, false)
  assert.equal(amended.finalScore, null)
})

test('citations, exact answer claims, explicit lists, IDs and reasons are validated', async () => {
  const f = fixture()
  const packet = await prepareReview(f.run, f.grading, f.review)
  for (const ref of [{ source: 'missing', quote: 'declined' }, { source: 'answer', quote: 'fabricated evidence' }, { source: 'answer', quote: ' ' }, { source: 'toString', quote: 'text' }]) {
    const decision = f.decision()
    decision.completion!.evidence = [ref]
    assert.throws(() => validateDecision(decision, packet))
  }
  const decision = f.decision()
  decision.claims = [{ id: 'c1', claim: 'Different claim', claimSource: { source: 'answer', quote: 'Inventory caused the failure.' }, classification: 'unverified', decision: 'Fail', evidence: [{ source: 'answer', quote: f.answer }], reason: 'Synthetic reason' }]
  assert.throws(() => validateDecision(decision, packet), /exactly/)
  decision.claims[0].claim = 'Inventory caused the failure.'
  decision.claims.push(decision.claims[0])
  assert.throws(() => validateDecision(decision, packet), /Duplicate claim/)
  assert.throws(() => validateDecision({ ...f.decision(), claims: null }, packet), /explicit array/)
  assert.throws(() => validateDecision({ ...f.decision(), findings: null }, packet), /explicit array/)
  assert.throws(() => validateDecision({ ...f.decision(), toolReviews: null }, packet), /explicit array/)
  assert.throws(() => validateDecision({ ...f.decision(), reviewer: ' ' }, packet), /nonempty/)
  assert.throws(() => validateDecision({ ...f.decision(), madeUp: true }, packet), /Unknown review field/)
})

test('ordinary failed tools require explicit disposition; a blocking failure prevents finalisation', async () => {
  const f = fixture('checkout-failure', true)
  const packet = await prepareReview(f.run, f.grading, f.review)
  assert.deepEqual(packet.failedTools, ['tool-event-2'])
  const decision = f.decision()
  recordReview(f.review, decision)
  assert.throws(() => signoffReview(f.review, 1, 'synthetic-human', true), /Failed tool needs review/)
  decision.toolReviews = [{ source: 'tool-event-2', disposition: 'blocking', reason: 'Synthetic blocker' }]
  recordReview(f.review, decision)
  assert.throws(() => signoffReview(f.review, 2, 'synthetic-human', true), /Blocking tool/)
  decision.toolReviews[0].disposition = 'not-blocking'
  recordReview(f.review, decision)
  assert.equal(signoffReview(f.review, 3, 'synthetic-human', true).finalScore, 1)
  decision.toolReviews[0].source = 'tool-event-1'
  assert.throws(() => validateDecision(decision, packet), /recorded failed tool/)
  decision.toolReviews[0].source = 'tool-event-2'
  decision.toolReviews.push(decision.toolReviews[0])
  assert.throws(() => validateDecision(decision, packet), /Duplicate tool/)
})

test('execution errors and harness blockers stay unscored, including malformed-grader errors with metadata', async () => {
  for (const kind of ['grader', 'investigator', 'harness']) {
    const f = fixture()
    if (kind === 'grader') { f.report.status = 'execution-error'; delete f.report.score; f.report.reason = 'Synthetic invalid grader output' }
    if (kind === 'investigator') {
      f.investigator.exit.code = 1
      fs.writeFileSync(path.join(f.evidence, 'metrics.json'), JSON.stringify(f.investigator))
      f.data.evidence = investigatorEvidence(f.investigator, 'checkout-failure', f.answer, f.run)
      f.report.investigatorEvidence = f.data.evidence
      f.prompt()
    }
    if (kind === 'harness') saveJson(path.join(f.run, 'harness-blocker.json'), { status: 'harness-blocker', evidence: f.evidence })
    f.writeReport()
    const packet = await prepareReview(f.run, f.grading, f.review)
    assert.ok(packet.unscoredReasons.length)
    assert.throws(() => recordReview(f.review, f.decision(0, 'correct')), /remain unscored/)
    const decision = f.decision()
    decision.completion = null
    assert.equal(recordReview(f.review, decision).finalScore, null)
    assert.throws(() => signoffReview(f.review, 1, 'synthetic-human', true), /Cannot sign off/)
  }
})

test('obsolete question/rubric reports are refused without writing review or modifying old evidence', async () => {
  for (const mismatch of ['question', 'rubric']) {
    const f = fixture()
    if (mismatch === 'question') f.data.question = 'Synthetic old compound question'
    else f.data.criteria = [{ id: 'reported-cause', text: 'Synthetic obsolete binary criterion' }] as typeof f.data.criteria
    f.prompt()
    const before = fingerprint(f.run)
    await assert.rejects(prepareReview(f.run, f.grading, f.review), /current-rubric/)
    assert.equal(fs.existsSync(f.review), false)
    assert.deepEqual(fingerprint(f.run), before)
  }
})

test('attempt identity, answer and saved grading evidence mismatches are rejected', async () => {
  for (const mismatch of ['task', 'session', 'answer', 'packet']) {
    const f = fixture()
    if (mismatch === 'task') f.investigator.task = 'cpu-evidence'
    if (mismatch === 'session') f.investigator.sessionIDs = ['another-session']
    if (mismatch === 'answer') fs.writeFileSync(path.join(f.evidence, 'answer.txt'), 'another answer')
    if (mismatch === 'packet') f.report.investigatorEvidence = {}
    f.writeReport()
    await assert.rejects(prepareReview(f.run, f.grading, f.review), /identity|mismatch|differs|session/i)
    assert.equal(fs.existsSync(f.review), false)
  }
})

test('source/snapshot changes and event-chain or source-packet tampering prevent final results', async () => {
  for (const target of ['source', 'snapshot', 'event', 'packet', 'gap']) {
    const f = fixture()
    const packet = await prepareReview(f.run, f.grading, f.review)
    recordReview(f.review, f.decision())
    if (target === 'source') fs.appendFileSync(path.join(f.evidence, 'answer.txt'), ' changed')
    if (target === 'snapshot') fs.appendFileSync(path.join(f.review, packet.sources[0].snapshot), ' changed')
    if (target === 'event') {
      const file = path.join(f.review, 'events/000001.json')
      const event = record(readJson(file))
      event.previousHash = 'not-the-previous-hash'
      fs.writeFileSync(file, JSON.stringify(event), { mode: 0o600 })
    }
    if (target === 'packet') fs.appendFileSync(path.join(f.review, 'source.json'), '\n')
    if (target === 'gap') fs.renameSync(path.join(f.review, 'events/000001.json'), path.join(f.review, 'events/000003.json'))
    assert.throws(() => signoffReview(f.review, 1, 'synthetic-human', true), /changed|mismatch|sequence/)
    assert.throws(() => showReview(f.review))
  }
})

test('path boundaries reject run writes, repository storage, outside/symlink evidence and symlink history', async () => {
  const f = fixture()
  await assert.rejects(prepareReview(f.run, f.grading, path.join(f.run, 'review')), /original run/)
  await assert.rejects(prepareReview(f.run, '../outside.json', f.review))
  await assert.rejects(prepareReview(f.run, path.join(f.run, f.grading), f.review), /relative/)
  const repository = path.join(f.parent, 'fake-repository')
  fs.mkdirSync(repository)
  fs.writeFileSync(path.join(repository, '.git'), 'synthetic git marker')
  await assert.rejects(prepareReview(f.run, f.grading, path.join(repository, 'review')), /outside repositories/)
  const metrics = path.join(f.evidence, 'metrics.json')
  fs.renameSync(metrics, metrics + '.real')
  fs.symlinkSync(metrics + '.real', metrics)
  await assert.rejects(prepareReview(f.run, f.grading, f.review), /symlink/)
  const safe = fixture()
  await prepareReview(safe.run, safe.grading, safe.review)
  fs.renameSync(path.join(safe.review, 'events'), path.join(safe.review, 'events-real'))
  fs.symlinkSync(path.join(safe.review, 'events-real'), path.join(safe.review, 'events'))
  assert.throws(() => recordReview(safe.review, safe.decision()), /symlink/)
})

test('the read-only consumer rejects altered signoff bindings and changes after finalisation', async () => {
  for (const target of ['reviewer', 'revision', 'decisionHash', 'sourceHash', 'original']) {
    const f = fixture()
    await prepareReview(f.run, f.grading, f.review)
    recordReview(f.review, f.decision())
    signoffReview(f.review, 1, 'synthetic-human', true)
    const before = fingerprint(f.review)
    assert.equal(showReview(f.review).finalScore, 1)
    assert.deepEqual(fingerprint(f.review), before)
    if (target === 'original') fs.appendFileSync(path.join(f.evidence, 'answer.txt'), ' changed after signoff')
    else {
      const file = path.join(f.review, 'events/000002.json')
      const event = record(readJson(file))
      event[target] = target === 'revision' ? 2 : 'altered binding'
      fs.writeFileSync(file, JSON.stringify(event), { mode: 0o600 })
    }
    assert.throws(() => showReview(f.review), /binding|mismatch|changed/)
  }
})

test('export score 1 accepts only saved successful proof with exact retained payload bytes and reported location', async () => {
  for (const archive of [false, true]) {
    const f = await exportFixture({ archive })
    const before = fingerprint(f.run)
    const packet = await prepareReview(f.run, f.grading, f.review)
    assert.equal(packet.exportFiles.length, 1)
    assert.equal(packet.exportFiles[0].file, path.join(f.workspace, f.file))
    recordReview(f.review, f.decision)
    assert.equal(signoffReview(f.review, 1, 'synthetic-human', true).finalScore, 1)
    assert.deepEqual(fingerprint(f.run), before)
    const invalid = structuredClone(f.decision)
    invalid.completion!.export!.location.quote = 'Exported to ' + f.file + '.'
    assert.throws(() => validateDecision(invalid, packet), /does not identify/)
    invalid.completion!.export!.file = path.join(f.workspace, 'different.json')
    assert.throws(() => validateDecision(invalid, packet), /matching retained replay proof/)
  }
})

test('failed, missing or mismatching export proof cannot be overridden to 1; score 0 remains reviewable', async () => {
  for (const options of [{ pass: false }, { proof: false }, { differentPayload: true }, { archive: true, wrongMember: true }]) {
    const f = await exportFixture(options)
    const packet = await prepareReview(f.run, f.grading, f.review)
    assert.equal(packet.exportFiles.length, 0)
    assert.throws(() => recordReview(f.review, f.decision), /matching retained replay proof/)
    f.decision.completion!.action = 'correct'
    f.decision.completion!.score = 0
    recordReview(f.review, f.decision)
    assert.equal(signoffReview(f.review, 1, 'synthetic-human', true).finalScore, 0)
  }
})

test('missing retained export and symlink export cannot supply a passing file', async () => {
  const missing = await exportFixture()
  fs.renameSync(path.join(missing.scratch, missing.file), path.join(missing.scratch, 'removed-by-synthetic-test.json'))
  const packet = await prepareReview(missing.run, missing.grading, missing.review)
  assert.deepEqual(packet.exportFiles, [])
  assert.throws(() => recordReview(missing.review, missing.decision), /matching retained replay proof/)
  const linked = await exportFixture()
  fs.renameSync(path.join(linked.scratch, linked.file), path.join(linked.scratch, 'real.json'))
  fs.symlinkSync(path.join(linked.scratch, 'real.json'), path.join(linked.scratch, linked.file))
  await assert.rejects(prepareReview(linked.run, linked.grading, linked.review), /symlink/)
})

test('export or retained replay mutation after preparation prevents signoff without replaying anything', async () => {
  for (const target of ['export', 'proof', 'payload']) {
    const f = await exportFixture()
    await prepareReview(f.run, f.grading, f.review)
    recordReview(f.review, f.decision)
    const file = target === 'export' ? path.join(f.scratch, f.file) : target === 'proof' ? path.join(f.run, 'export-replay-synthetic/export-proof.json') : path.join(f.run, 'export-replay-synthetic/fixture/trace.json')
    fs.appendFileSync(file, ' changed')
    assert.throws(() => signoffReview(f.review, 1, 'synthetic-human', true), /Original source changed/)
  }
})

test('CLI prepare/record/signoff/show uses explicit confirmation and keeps all run bytes unchanged', () => {
  const f = fixture()
  const before = fingerprint(f.run)
  const cli = (...args: string[]) => spawnSync(process.execPath, [path.join(import.meta.dirname, 'human-review.ts'), ...args], { encoding: 'utf8' })
  const prepared = cli('prepare', '--run', f.run, '--grading', f.grading, '--review-dir', f.review)
  assert.equal(prepared.status, 0, prepared.stderr)
  assert.equal(record(JSON.parse(prepared.stdout)).status, 'pending')
  const draft = path.join(f.review, 'draft.json')
  fs.writeFileSync(draft, JSON.stringify(f.decision()), { mode: 0o600 })
  const recorded = cli('record', '--review-dir', f.review, '--decision', draft)
  assert.equal(recorded.status, 0, recorded.stderr)
  assert.equal(record(JSON.parse(recorded.stdout)).status, 'pending')
  assert.equal(cli('signoff', '--review-dir', f.review, '--revision', '1', '--reviewer', 'synthetic-human').status, 1)
  const signed = cli('signoff', '--review-dir', f.review, '--revision', '1', '--reviewer', 'synthetic-human', '--confirm-final')
  assert.equal(signed.status, 0, signed.stderr)
  const shown = cli('show', '--review-dir', f.review)
  assert.equal(shown.status, 0, shown.stderr)
  assert.equal(record(JSON.parse(shown.stdout)).finalScore, 1)
  assert.equal(cli('show', '--review-dir', f.review, '--unexpected', 'value').status, 1)
  assert.equal(cli('show', '--review-dir', f.review, '--review-dir', f.review).status, 1)
  fs.writeFileSync(draft, 'invalid-json-with-synthetic-private-string')
  const invalid = cli('record', '--review-dir', f.review, '--decision', draft)
  assert.equal(invalid.status, 1)
  assert.equal(invalid.stderr.trim(), 'Invalid review JSON')
  assert.deepEqual(fingerprint(f.run), before)
})

test('pure signoff blockers show question mismatch and missing completion without inventing scores', async () => {
  const f = fixture()
  const packet = await prepareReview(f.run, f.grading, f.review)
  const decision = f.decision()
  decision.completion = null
  assert.deepEqual(signoffBlockers({ ...packet, originalQuestion: 'different' }, decision), ['Original question differs from the approved question', 'Completion decision is missing'])
})
