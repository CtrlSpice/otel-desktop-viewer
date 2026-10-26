import fs from 'node:fs'
import path from 'node:path'
import { investigatorEvidence, ownedPath, sha256, toolEvidence } from './evidence.ts'
import { exportPayloads } from './verify-export.ts'
import { record, saveJson, SUITE } from './runtime.ts'

type Citation = { source: string; quote: string }
type Completion = {
  action: 'accept' | 'correct'
  score: number
  reason: string
  evidence: Citation[]
  export?: { file: string; location: Citation }
}
export type HumanDecision = {
  schemaVersion: 1
  reviewer: string
  completion: Completion | null
  claims: {
    id: string
    claim: string
    claimSource: Citation
    classification: 'supported' | 'contradicted' | 'unverified'
    decision: 'Pass' | 'Fail' | null
    evidence: Citation[]
    reason: string
  }[]
  findings: { finding: string; evidence: Citation[]; reason: string }[]
  toolReviews: { source: string; disposition: 'not-blocking' | 'blocking'; reason: string }[]
}
type Source = { id: string; origin: string; snapshot: string; sha256: string }
export type ReviewPacket = {
  schemaVersion: 1
  run: string
  taskID: string
  investigator: { evidence: string; sessionIDs: string[]; model: string }
  originalAutomated: Record<string, unknown>
  originalQuestion: string
  approvedQuestion: string
  approvedRubric: { criteria: { id: string; scoring: { Fail: string; Partial?: string; Pass: string } }[] }
  unscoredReasons: string[]
  failedTools: string[]
  references: Record<string, string>
  exportFiles: { file: string; proof: string }[]
  sources: Source[]
}
type ReviewEvent = {
  type: 'record' | 'signoff'
  revision: number
  previousHash: string | null
  sourceHash: string
  recordedAt: string
  decision?: HumanDecision
  reviewer?: string
  decisionHash?: string
}

const text = (value: unknown): string => {
  if (typeof value !== 'string' || !value.trim()) throw new Error('Expected nonempty text')
  return value
}
const list = (value: unknown): unknown[] => {
  if (!Array.isArray(value)) throw new Error('Expected an explicit array')
  return value
}
const fields = (value: unknown, allowed: string[]) => {
  const item = record(value)
  if (Object.keys(item).some(key => !allowed.includes(key))) throw new Error('Unknown review field')
  return item
}
const digest = (value: unknown) => sha256(JSON.stringify(value))

export function validateDecision(value: unknown, packet: ReviewPacket): HumanDecision {
  const item = fields(value, ['schemaVersion', 'reviewer', 'completion', 'claims', 'findings', 'toolReviews'])
  if (item.schemaVersion !== 1) throw new Error('Unknown review schema')
  const cite = (value: unknown): Citation => {
    const ref = fields(value, ['source', 'quote'])
    const source = text(ref.source)
    const quote = text(ref.quote)
    if (!Object.hasOwn(packet.references, source) || !packet.references[source].includes(quote))
      throw new Error('Unknown source or quote absent from source')
    return { source, quote }
  }
  const evidence = (value: unknown) => {
    const refs = list(value).map(cite)
    if (!refs.length) throw new Error('Evidence is required')
    return refs
  }
  let completion: Completion | null = null
  if (item.completion !== null) {
    if (packet.unscoredReasons.length) throw new Error('Execution errors and harness blockers remain unscored')
    const result = fields(item.completion, ['action', 'score', 'reason', 'evidence', 'export'])
    if (result.action !== 'accept' && result.action !== 'correct') throw new Error('Choose accept or correct')
    const scoring = packet.approvedRubric.criteria[0].scoring
    if (result.score !== 0 && result.score !== 1 && !(result.score === 0.5 && scoring.Partial))
      throw new Error('Score is not defined by the approved task rubric')
    if (result.action === 'accept' && result.score !== packet.originalAutomated.score)
      throw new Error('Accept must preserve the original automated score')
    completion = { action: result.action, score: result.score, reason: text(result.reason), evidence: evidence(result.evidence) }
    if (result.export !== undefined) {
      if (packet.taskID !== 'trace-export') throw new Error('Export selection belongs only to the export task')
      const selected = fields(result.export, ['file', 'location'])
      const location = cite(selected.location)
      const file = text(selected.file)
      // A literal answer quote, not an answer-supplied path to read.
      if (location.source !== 'answer') throw new Error('Export location must quote the answer')
      completion.export = { file, location }
    }
    if (packet.taskID === 'trace-export' && result.score === 1) {
      const selected = completion.export
      const workspace = record(packet.originalAutomated.investigator).workspace
      const artifact = packet.originalAutomated.artifact === undefined ? {} : record(packet.originalAutomated.artifact)
      if (artifact.pass !== true || !Array.isArray(artifact.files) || typeof workspace !== 'string' || !selected || !artifact.files.includes(selected.file) || !packet.exportFiles.some(proof => proof.file === selected.file))
        throw new Error('Export score 1 requires a saved successful check and matching retained replay proof')
      const relative = path.relative(workspace, selected.file)
      if (![selected.file, relative, './' + relative].includes(selected.location.quote))
        throw new Error('Reported location does not identify the selected verified file')
    }
  }
  const claims = list(item.claims).map((value): HumanDecision['claims'][number] => {
    const claim = fields(value, ['id', 'claim', 'claimSource', 'classification', 'decision', 'evidence', 'reason'])
    const claimSource = cite(claim.claimSource)
    const exactClaim = text(claim.claim)
    if (claimSource.source !== 'answer' || claimSource.quote !== exactClaim) throw new Error('Claim must quote the answer exactly')
    if (claim.classification !== 'supported' && claim.classification !== 'contradicted' && claim.classification !== 'unverified')
      throw new Error('Unknown claim classification')
    if (claim.decision !== 'Pass' && claim.decision !== 'Fail' && claim.decision !== null) throw new Error('Unknown claim decision')
    if (claim.classification === 'contradicted' && claim.decision === 'Pass') throw new Error('A contradicted claim cannot receive Pass')
    return { id: text(claim.id), claim: exactClaim, claimSource, classification: claim.classification, decision: claim.decision, evidence: evidence(claim.evidence), reason: text(claim.reason) }
  })
  if (new Set(claims.map(claim => claim.id)).size !== claims.length) throw new Error('Duplicate claim ID')
  const findings = list(item.findings).map(value => {
    const finding = fields(value, ['finding', 'evidence', 'reason'])
    return { finding: text(finding.finding), evidence: evidence(finding.evidence), reason: text(finding.reason) }
  })
  const toolReviews = list(item.toolReviews).map((value): HumanDecision['toolReviews'][number] => {
    const review = fields(value, ['source', 'disposition', 'reason'])
    const source = text(review.source)
    if (!packet.failedTools.includes(source)) throw new Error('Tool review must identify a recorded failed tool')
    if (review.disposition !== 'not-blocking' && review.disposition !== 'blocking') throw new Error('Unknown tool disposition')
    return { source, disposition: review.disposition, reason: text(review.reason) }
  })
  if (new Set(toolReviews.map(review => review.source)).size !== toolReviews.length) throw new Error('Duplicate tool review')
  return { schemaVersion: 1, reviewer: text(item.reviewer), completion, claims, findings, toolReviews }
}

export function signoffBlockers(packet: ReviewPacket, decision: HumanDecision) {
  return [
    ...packet.unscoredReasons,
    ...(packet.originalQuestion !== packet.approvedQuestion ? ['Original question differs from the approved question'] : []),
    ...(!decision.completion ? ['Completion decision is missing'] : []),
    ...(decision.claims.some(claim => claim.decision === null) ? ['Claim decisions remain unresolved'] : []),
    ...packet.failedTools.filter(source => !decision.toolReviews.some(review => review.source === source)).map(source => 'Failed tool needs review: ' + source),
    ...decision.toolReviews.filter(review => review.disposition === 'blocking').map(review => 'Blocking tool failure: ' + review.source),
  ]
}

export function reviewState(packet: ReviewPacket, events: ReviewEvent[]) {
  const latest = events.filter(event => event.type === 'record').at(-1)
  const signed = events.at(-1)
  const final = !!latest && signed?.type === 'signoff' && signed.revision === latest.revision
  const claims = latest?.decision?.claims
  return {
    run: packet.run,
    taskID: packet.taskID,
    investigator: packet.investigator,
    originalAutomated: packet.originalAutomated,
    originalQuestion: packet.originalQuestion,
    approvedQuestion: packet.approvedQuestion,
    approvedRubric: packet.approvedRubric,
    status: final ? 'final' : 'pending',
    revision: latest?.revision ?? null,
    human: latest?.decision ?? null,
    latestRevisionSigned: final,
    signedOff: final,
    finalScore: final ? latest?.decision?.completion?.score ?? null : null,
    claimCounts: claims ? {
      supported: claims.filter(claim => claim.classification === 'supported').length,
      contradicted: claims.filter(claim => claim.classification === 'contradicted').length,
      unverified: claims.filter(claim => claim.classification === 'unverified').length,
    } : null,
    signoff: final ? signed : null,
    unscoredReasons: packet.unscoredReasons,
    failedTools: packet.failedTools,
    blockers: latest?.decision ? signoffBlockers(packet, latest.decision) : ['Human decision has not been recorded'],
    identityNotice: 'Reviewer identity is self-declared, not authenticated or cryptographically signed.',
    claimNotice: 'Human review is not automated exhaustive claim coverage. Unverified Pass means acceptance, not proof.',
  }
}

function externalDirectory(supplied: string) {
  if (!path.isAbsolute(supplied)) throw new Error('Use an absolute external directory')
  const root = fs.realpathSync(supplied)
  if (!fs.statSync(root).isDirectory()) throw new Error('Expected a directory')
  for (let parent = root; ; parent = path.dirname(parent)) {
    if (fs.existsSync(path.join(parent, '.git'))) throw new Error('Human review data must stay outside repositories')
    if (parent === path.dirname(parent)) break
  }
  return root
}

function fileBytes(root: string, supplied: string) {
  const file = ownedPath(root, supplied)
  const fd = fs.openSync(file, fs.constants.O_RDONLY | fs.constants.O_NOFOLLOW)
  try {
    if (!fs.fstatSync(fd).isFile()) throw new Error('Evidence must be a regular file')
    return { file, bytes: fs.readFileSync(fd) }
  } finally {
    fs.closeSync(fd)
  }
}

function promptData(prompt: string) {
  const line = prompt.split('\n').find(line => line.startsWith('{"question"'))
  if (!line) throw new Error('Missing original grading question/rubric')
  return record(JSON.parse(line))
}

export async function prepareReview(runDirectory: string, grading: string, reviewDirectory: string) {
  const run = externalDirectory(runDirectory)
  if (path.isAbsolute(grading)) throw new Error('Grading path must be relative to the run')
  const reportFile = fileBytes(run, path.resolve(run, grading))
  const report = record(JSON.parse(reportFile.bytes.toString('utf8')))
  const taskID = text(report.taskID)
  const tasksFile = fileBytes(SUITE, path.join(SUITE, 'tasks.json'))
  const rubricFile = fileBytes(SUITE, path.join(SUITE, 'rubrics.json'))
  const task = list(JSON.parse(tasksFile.bytes.toString('utf8'))).map(record).find(task => task.id === taskID)
  const rubric = record(record(JSON.parse(rubricFile.bytes.toString('utf8')))[taskID])
  if (!task || !Array.isArray(rubric.criteria) || rubric.criteria.length !== 1) throw new Error('Unknown approved task rubric')
  const criterion = record(rubric.criteria[0])
  const scoring = record(criterion.scoring)
  text(scoring.Fail)
  text(scoring.Pass)
  if (scoring.Partial !== undefined) text(scoring.Partial)
  const investigator = record(report.investigator)
  const evidence = ownedPath(run, investigator.evidence)
  const grader = record(report.metadata)
  const graderEvidence = ownedPath(run, grader.evidence)
  if (report.role !== 'grader' || report.provisional !== true || investigator.role !== 'investigator' || investigator.task !== taskID || grader.role !== 'grader' || grader.task !== taskID ||
      path.dirname(evidence) !== path.join(run, 'runs') || !path.basename(evidence).startsWith('investigator-') ||
      path.dirname(graderEvidence) !== path.join(run, 'runs') || !path.basename(graderEvidence).startsWith('grader-') ||
      reportFile.file !== path.join(graderEvidence, 'grading.json')) throw new Error('Grading/attempt identity mismatch')
  const originalPrompt = fileBytes(run, path.join(graderEvidence, 'prompt.txt'))
  const originalData = promptData(originalPrompt.bytes.toString('utf8'))
  if (originalData.question !== task.goal || digest(originalData.criteria) !== digest(rubric.criteria) || originalData.reference !== rubric.reference)
    throw new Error('Review requires a current-question, current-rubric grading report; obsolete scores are not supported')
  const unscoredReasons: string[] = []
  if (report.status === 'execution-error') unscoredReasons.push('Original grader execution error')
  else if (!['pass', 'partial', 'fail'].includes(String(report.status)) || typeof report.score !== 'number' || ![0, 0.5, 1].includes(report.score))
    throw new Error('Invalid saved automated result')
  const selected: { id: string; file: string; bytes: Buffer; binary?: boolean }[] = []
  const add = (id: string, supplied: string, binary = false) => {
    const source = fileBytes(run, supplied)
    selected.push({ id, ...source, binary })
    return source.bytes
  }
  selected.push({ id: 'grading', ...reportFile }, { id: 'grading-prompt', ...originalPrompt }, { id: 'approved-tasks', ...tasksFile }, { id: 'approved-rubrics', ...rubricFile })
  for (const name of ['metrics.json', 'answer.txt', 'stdout.jsonl', 'commands.json']) {
    const file = path.join(graderEvidence, name)
    if (fs.existsSync(file)) {
      const bytes = add('grader-' + name, file)
      if (name === 'metrics.json' && digest(JSON.parse(bytes.toString('utf8'))) !== digest(grader)) throw new Error('Grader metadata identity mismatch')
    }
  }
  if (investigator.harnessBlocker === true || grader.harnessBlocker === true) unscoredReasons.push('Original harness blocker')
  for (const [id, directory] of [['investigator-failure', evidence], ['grader-failure', graderEvidence]]) {
    if (fs.existsSync(path.join(directory, 'failure.json'))) {
      add(id, path.join(directory, 'failure.json'))
      unscoredReasons.push('Original execution failure: ' + id)
    }
  }
  const blockerPath = path.join(run, 'harness-blocker.json')
  if (fs.existsSync(blockerPath)) {
    const blocker = record(JSON.parse(fileBytes(run, blockerPath).bytes.toString('utf8')))
    if ([evidence, graderEvidence].some(directory => typeof blocker.evidence === 'string' && fs.realpathSync(blocker.evidence) === directory)) {
      add('harness-blocker', blockerPath)
      unscoredReasons.push('Original harness blocker')
    }
  }
  const answer = add('answer', path.join(evidence, 'answer.txt')).toString('utf8')
  const references: Record<string, string> = { answer }
  const failedTools: string[] = []
  if (!unscoredReasons.length) {
    const packet = investigatorEvidence(investigator, taskID, answer, run)
    const saved = record(report.investigatorEvidence)
    if (digest(packet) !== digest(saved)) throw new Error('Retained investigator evidence differs from saved grading evidence')
    if (originalData.answer !== answer || digest(originalData.evidence) !== digest(packet)) throw new Error('Grading prompt evidence/answer mismatch')
    for (const name of Object.keys(packet.sources)) if (name !== 'answer.txt') add('investigator-' + name, path.join(evidence, name))
    for (const [index, event] of packet.events.entries()) {
      const id = 'tool-event-' + (index + 1)
      references[id] = event.raw
      if (event.failures.length) failedTools.push(id)
    }
    const exit = record(packet.sessionExit)
    if (exit.code !== 0 || exit.timedOut === true) unscoredReasons.push('Original investigator execution error')
  } else {
    // Failed attempts may have incomplete tools. Preserve their raw transcript;
    // completed tool events still get references, but no score can be assigned.
    for (const name of ['metrics.json', 'stdout.jsonl', 'commands.json', 'prompt.txt']) {
      const file = path.join(evidence, name)
      if (fs.existsSync(file)) add('investigator-' + name, file)
    }
    const stdout = selected.find(source => source.id === 'investigator-stdout.jsonl')
    if (stdout) {
      try {
        for (const [index, event] of toolEvidence(stdout.bytes.toString('utf8'), list(investigator.sessionIDs).map(text)).entries()) {
          const id = 'tool-event-' + (index + 1)
          references[id] = event.raw
          if (event.failures.length) failedTools.push(id)
        }
      } catch {
        unscoredReasons.push('Incomplete tool evidence; inspect retained raw transcript')
      }
    }
  }
  const exportFiles: ReviewPacket['exportFiles'] = []
  if (taskID === 'trace-export' && report.artifact !== undefined) {
    const artifact = record(report.artifact)
    if (artifact.pass === true) {
      const workspace = text(investigator.workspace)
      const scratch = ownedPath(run, investigator.archivedWorkspace ?? investigator.workspace)
      const isolation = ownedPath(run, investigator.isolationRoot)
      if (scratch !== path.join(isolation, 'scratch')) throw new Error('Export scratch identity mismatch')
      for (const [index, value] of list(artifact.files).entries()) {
        const file = text(value)
        const suffix = path.relative(workspace, file)
        if (!path.isAbsolute(file) || suffix.startsWith('..' + path.sep) || suffix === '..' || path.isAbsolute(suffix) || !suffix) throw new Error('Verified export path outside investigator workspace')
        const retained = path.join(scratch, suffix)
        // Missing retained files/proofs block export score 1, but still permit a score 0 review.
        if (!fs.existsSync(retained)) continue
        const bytes = add('export-' + (index + 1), retained, true)
        let payloads: Awaited<ReturnType<typeof exportPayloads>>
        try { payloads = await exportPayloads(bytes) } catch { continue }
        for (const name of fs.readdirSync(run).filter(name => name.startsWith('export-replay-'))) {
          const proofPath = path.join(run, name, 'export-proof.json')
          const payloadPath = path.join(run, name, 'fixture/trace.json')
          if (!fs.existsSync(proofPath) || !fs.existsSync(payloadPath)) continue
          const proofBytes = fileBytes(run, proofPath)
          const proof = record(JSON.parse(proofBytes.bytes.toString('utf8')))
          if (proof.pass !== true || proof.retainedSpans !== 6 || (proof.file !== retained && proof.file !== file)) continue
          const delivered = fileBytes(run, payloadPath)
          if (!payloads.some(payload => payload.member === proof.member && payload.bytes.equals(delivered.bytes))) continue
          const id = 'export-proof-' + (exportFiles.length + 1)
          selected.push({ id, ...proofBytes }, { id: id + '-payload', ...delivered, binary: true })
          exportFiles.push({ file, proof: id })
          break
        }
      }
    }
  }
  const sessionIDs = list(investigator.sessionIDs).map(text)
  if (!sessionIDs.length) throw new Error('Missing investigator session identity')
  if (originalData.question === undefined || originalData.criteria === undefined) throw new Error('Missing original grading rubric')
  if (!path.isAbsolute(reviewDirectory)) throw new Error('Use an absolute external review directory')
  const parent = externalDirectory(path.dirname(reviewDirectory))
  const review = path.join(parent, path.basename(reviewDirectory))
  if (review === run || review.startsWith(run + path.sep)) throw new Error('Review directory cannot write into the original run')
  if (fs.existsSync(review)) throw new Error('Review directory exists; refusing to overwrite')
  const sources = selected.map((source, index) => ({ id: source.id, origin: source.file, snapshot: 'sources/' + String(index + 1).padStart(6, '0'), sha256: sha256(source.bytes) }))
  for (const source of selected) if (!source.binary) references[source.id] = source.bytes.toString('utf8')
  const packet: ReviewPacket = {
    schemaVersion: 1, run, taskID,
    investigator: { evidence, sessionIDs, model: text(investigator.model) },
    originalAutomated: report,
    originalQuestion: text(originalData.question),
    approvedQuestion: text(task.goal),
    approvedRubric: rubric as ReviewPacket['approvedRubric'],
    unscoredReasons, failedTools, references, exportFiles, sources,
  }
  fs.mkdirSync(review, { mode: 0o700 })
  fs.mkdirSync(path.join(review, 'sources'), { mode: 0o700 })
  fs.mkdirSync(path.join(review, 'events'), { mode: 0o700 })
  selected.forEach((source, index) => fs.writeFileSync(path.join(review, sources[index].snapshot), source.bytes, { flag: 'wx', mode: 0o600 }))
  saveJson(path.join(review, 'source.json'), packet)
  saveJson(path.join(review, 'draft.json'), { schemaVersion: 1, reviewer: null, completion: { action: null, score: null, reason: null, evidence: null }, claims: null, findings: null, toolReviews: null })
  fs.writeFileSync(path.join(review, 'audit.md'), [
    '# Local human review packet', '',
    'Pending: no human decisions or signoff have been recorded.',
    'Review the recorded answer. This is not automated exhaustive claim coverage.',
    'Reviewer labels are self-declared, not authentication or cryptographic signatures.',
    'Claims/findings/toolReviews start null; enter explicit arrays (including [] when appropriate).',
    'A source reference and literal quote establish a citation, not the correctness of its interpretation.',
    '', '```json', JSON.stringify(packet, null, 2), '```', '',
  ].join('\n'), { flag: 'wx', mode: 0o600 })
  return packet
}

function loadReview(directory: string) {
  const root = externalDirectory(directory)
  if (fs.statSync(root).mode & 0o077) throw new Error('Review directory must be private (0700)')
  const readPrivate = (name: string) => {
    const result = fileBytes(root, path.join(root, name))
    if (fs.statSync(result.file).mode & 0o077) throw new Error('Review files must be private (0600)')
    return result.bytes
  }
  const sourceBytes = readPrivate('source.json')
  const packet = JSON.parse(sourceBytes.toString('utf8')) as ReviewPacket
  if (packet.schemaVersion !== 1 || !Array.isArray(packet.sources)) throw new Error('Invalid review packet')
  for (const source of packet.sources) {
    if (sha256(readPrivate(source.snapshot)) !== source.sha256) throw new Error('Review snapshot changed: ' + source.id)
    const origin = source.origin
    const originRoot = source.id === 'approved-tasks' || source.id === 'approved-rubrics' ? SUITE : externalDirectory(packet.run)
    if (sha256(fileBytes(originRoot, origin).bytes) !== source.sha256) throw new Error('Original source changed: ' + source.id)
  }
  const sourceHash = sha256(sourceBytes)
  const names = fs.readdirSync(ownedPath(root, path.join(root, 'events'))).sort()
  const events: ReviewEvent[] = []
  let previousHash: string | null = null
  let revision = 0
  let latest: HumanDecision | undefined
  for (const [index, name] of names.entries()) {
    if (name !== String(index + 1).padStart(6, '0') + '.json') throw new Error('Invalid review event sequence')
    const bytes = readPrivate('events/' + name)
    const event = JSON.parse(bytes.toString('utf8')) as ReviewEvent
    fields(event, event.type === 'record'
      ? ['type', 'revision', 'previousHash', 'sourceHash', 'recordedAt', 'decision']
      : ['type', 'revision', 'previousHash', 'sourceHash', 'recordedAt', 'reviewer', 'decisionHash'])
    if (event.previousHash !== previousHash || event.sourceHash !== sourceHash || typeof event.recordedAt !== 'string' || !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$/.test(event.recordedAt)) throw new Error('Review event hash/time mismatch')
    if (event.type === 'record') {
      if (event.revision !== revision + 1) throw new Error('Invalid correction revision')
      latest = validateDecision(event.decision, packet)
      if (digest(latest) !== digest(event.decision)) throw new Error('Recorded decision changed')
      revision = event.revision
    } else if (event.type === 'signoff') {
      if (!latest || event.revision !== revision || event.decisionHash !== digest(latest) || event.reviewer !== latest.reviewer || signoffBlockers(packet, latest).length) throw new Error('Invalid signoff binding')
    } else throw new Error('Unknown review event')
    events.push(event)
    previousHash = sha256(bytes)
  }
  return { root, packet, sourceHash, events, previousHash, revision, latest }
}

function appendEvent(review: ReturnType<typeof loadReview>, event: ReviewEvent) {
  saveJson(path.join(review.root, 'events', String(review.events.length + 1).padStart(6, '0') + '.json'), event)
  return reviewState(review.packet, [...review.events, event])
}

export function recordReview(directory: string, value: unknown) {
  const review = loadReview(directory)
  const decision = validateDecision(value, review.packet)
  return appendEvent(review, { type: 'record', revision: review.revision + 1, previousHash: review.previousHash, sourceHash: review.sourceHash, recordedAt: new Date().toISOString(), decision })
}

export function signoffReview(directory: string, revision: number, reviewer: string, confirmFinal: boolean) {
  const review = loadReview(directory)
  if (!confirmFinal || !Number.isSafeInteger(revision) || revision !== review.revision || !review.latest || text(reviewer) !== review.latest.reviewer) throw new Error('Signoff requires confirmation, the current revision and its reviewer')
  const blockers = signoffBlockers(review.packet, review.latest)
  if (blockers.length) throw new Error('Cannot sign off: ' + blockers.join('; '))
  return appendEvent(review, { type: 'signoff', revision, previousHash: review.previousHash, sourceHash: review.sourceHash, recordedAt: new Date().toISOString(), reviewer, decisionHash: digest(review.latest) })
}

export function showReview(directory: string) {
  const review = loadReview(directory)
  return reviewState(review.packet, review.events)
}

async function main(args: string[]) {
  const [command, ...flags] = args
  const options: Record<string, string> = {}
  let confirmFinal = false
  for (let index = 0; index < flags.length; index++) {
    const flag = flags[index]
    if (flag === '--confirm-final' && !confirmFinal) { confirmFinal = true; continue }
    if (!flag.startsWith('--') || Object.hasOwn(options, flag) || !flags[index + 1] || flags[index + 1].startsWith('--')) throw new Error('Invalid or duplicate review option')
    options[flag] = flags[++index]
  }
  const allowed: Record<string, string[]> = {
    prepare: ['--run', '--grading', '--review-dir'], record: ['--review-dir', '--decision'],
    signoff: ['--review-dir', '--revision', '--reviewer'], show: ['--review-dir'],
  }
  if (!allowed[command] || Object.keys(options).some(key => !allowed[command].includes(key)) || allowed[command].some(key => !options[key]) || (confirmFinal && command !== 'signoff')) throw new Error('Use prepare, record, signoff or show with their documented options')
  const directory = options['--review-dir']
  if (command === 'prepare') {
    const packet = await prepareReview(options['--run'], options['--grading'], directory)
    return { status: 'pending', taskID: packet.taskID, reviewDirectory: directory, draft: path.join(directory, 'draft.json') }
  }
  if (command === 'record') {
    const file = options['--decision']
    const root = externalDirectory(path.dirname(file))
    const source = fileBytes(root, file)
    if (fs.statSync(source.file).mode & 0o077) throw new Error('Decision input must be private (0600)')
    return recordReview(directory, JSON.parse(source.bytes.toString('utf8')))
  }
  if (command === 'signoff') {
    if (!/^[1-9]\d*$/.test(options['--revision'])) throw new Error('Revision must be a positive integer')
    return signoffReview(directory, Number(options['--revision']), options['--reviewer'], confirmFinal)
  }
  return showReview(directory)
}

if (process.argv[1] && fs.realpathSync(process.argv[1]) === import.meta.filename) {
  try {
    console.log(JSON.stringify(await main(process.argv.slice(2)), null, 2))
  } catch (error) {
    // JSON syntax diagnostics can contain supplied text; do not print those bytes.
    console.error(error instanceof SyntaxError ? 'Invalid review JSON' : error instanceof Error ? error.message : 'Human review failed')
    process.exitCode = 1
  }
}
