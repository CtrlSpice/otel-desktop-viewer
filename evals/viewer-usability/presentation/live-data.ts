import fs from 'node:fs'
import path from 'node:path'
import { isDeepStrictEqual } from 'node:util'
import { aggregate } from './aggregate.ts'
import { modelSummaries } from '../model-summary.ts'
import { pricingSnapshot } from '../pricing.ts'
import { record } from '../runtime.ts'
import { modelExecutionErrors } from '../execution-errors.ts'

export type LiveState = 'waiting' | 'running' | 'stopped' | 'complete' | 'unavailable'
export type LiveSelection = { manifest: string; runs: string[] }

function read(file: string): unknown {
  try {
    return JSON.parse(fs.readFileSync(file, 'utf8'))
  } catch (error) {
    if (error instanceof Error && 'code' in error && error.code === 'ENOENT') return undefined
    throw error
  }
}

function records(value: unknown): Record<string, unknown>[] {
  if (!Array.isArray(value)) throw new Error('Expected saved result records')
  return value.map(record)
}

function attemptMetrics(root: string, evidence: unknown) {
  if (typeof evidence !== 'string') throw new Error('Missing execution-error evidence')
  const directory = fs.realpathSync(evidence)
  const relative = path.relative(fs.realpathSync(root), directory)
  if (relative.startsWith('..') || path.isAbsolute(relative)) throw new Error('Error evidence must belong to the selected run')
  const value = read(path.join(directory, 'metrics.json')) ?? read(path.join(directory, 'failure.json'))
  return record(value)
}

function gradeRow(report: Record<string, unknown>) {
  const investigator = record(report.investigator)
  record(report.metadata)
  if (!['pass', 'partial', 'fail', 'execution-error'].includes(String(report.status)))
    throw new Error('Incomplete grading report')
  return {
    provider: { label: investigator.model },
    vars: { eval_model: investigator.model, task_id: report.taskID },
    success: report.status === 'pass',
    failureReason: report.status === 'execution-error' ? 2 : report.status === 'pass' ? 0 : 1,
    response: { metadata: investigator },
    gradingResult: { score: report.status === 'execution-error' ? null : report.score, componentResults: [{ metadata: report }] },
  }
}

function readRun(root: string) {
  const manifest = record(read(path.join(root, 'run-manifest.json')))
  const pricing = read(path.join(root, 'pricing-snapshot.json'))
  const attemptsDirectory = path.join(root, 'runs')
  const attempts = fs.existsSync(attemptsDirectory)
    ? fs.readdirSync(attemptsDirectory, { withFileTypes: true }).filter(entry => entry.isDirectory()).map(entry => path.join(attemptsDirectory, entry.name))
    : []
  const reports = attempts.flatMap(directory => {
    const report = read(path.join(directory, 'grading.json'))
    return report === undefined ? [] : [record(report)]
  })
  const pilot = read(path.join(root, 'pilot.json'))
  const rows: Record<string, unknown>[] = pilot === undefined
    ? reports.map(gradeRow)
    : records(record(record(pilot).results).results)
  const errorValue = read(path.join(root, 'execution-error.json')) ?? read(path.join(root, 'harness-blocker.json'))
  const errors = [...modelExecutionErrors(root), ...(errorValue === undefined ? [] : [record(errorValue)])]
  if (pilot === undefined) for (const error of errors) {
    const metadata = attemptMetrics(root, error.evidence)
    const alreadyRecorded = rows.some(row => record(record(row.response).metadata).evidence === metadata.evidence)
    if (!alreadyRecorded) rows.push({
      provider: { label: metadata.model ?? error.model },
      vars: { eval_model: metadata.model ?? error.model, task_id: metadata.task ?? error.task },
      success: false, failureReason: 2,
      response: { error: 'Recorded execution error', metadata: { ...metadata, harnessBlocker: metadata.harnessBlocker === true || error.status === 'harness-blocker' } },
    })
  }
  // Completed pilots can omit investigator metadata after a grading failure.
  const retainedInvestigators = pilot === undefined ? [] : attempts.flatMap(directory => {
    if (!path.basename(directory).startsWith('investigator-')) return []
    const value = read(path.join(directory, 'metrics.json')) ?? read(path.join(directory, 'failure.json'))
    return value === undefined ? [] : [record(value)]
  })
  const investigators = retainedInvestigators.length
    ? retainedInvestigators.map(metadata => ({ provider: { label: metadata.model }, response: { metadata } }))
    : rows
  const completion = read(path.join(root, 'run-completion.json')) ?? read(path.join(root, 'overnight-completion.json')) ?? read(path.join(root, 'pilot-completion.json'))
  const failure = read(path.join(root, 'run-failure.json')) ?? read(path.join(root, 'overnight-failure.json'))
  const stopped = errorValue !== undefined || failure !== undefined || (completion !== undefined && record(completion).exit !== 0)
  const state: LiveState = stopped ? 'stopped' : completion !== undefined ? 'complete'
    : fs.existsSync(path.join(root, 'launch.json')) || attempts.length > 0 ? 'running' : 'waiting'
  return { root, manifest, pricing, rows, reports, investigators, state }
}

export function liveRunDirectories(roots: string[]) {
  const found = new Set<string>()
  for (const value of roots) {
    const root = fs.realpathSync(value)
    if (fs.existsSync(path.join(root, 'run-manifest.json'))) found.add(root)
    else for (const entry of fs.readdirSync(root, { withFileTypes: true })) {
      if (!entry.isDirectory()) continue
      const directory = path.join(root, entry.name)
      if (fs.existsSync(path.join(directory, 'run-manifest.json'))) found.add(fs.realpathSync(directory))
    }
  }
  return [...found]
}

export function readLiveData(selection: LiveSelection) {
  const manifest = record(read(selection.manifest))
  const runs = liveRunDirectories(selection.runs).map(readRun)
  const prices = read(path.join(path.dirname(selection.manifest), 'pricing-snapshot.json')) ?? runs.find(run => run.pricing !== undefined)?.pricing
  if (!Array.isArray(manifest.models)) throw new Error('Select a plan with declared models')
  const plannedModels = manifest.models
  for (const run of runs) {
    if (run.pricing !== undefined && !isDeepStrictEqual(run.pricing, prices)) throw new Error('Selected runs have different pricing snapshots')
    for (const key of ['sourceSHA256', 'viewerSourceCommit', 'evalSourceCommit']) {
      if (manifest[key] !== undefined && run.manifest[key] !== undefined && manifest[key] !== run.manifest[key])
        throw new Error('Selected runs use different source versions')
    }
    if (!Array.isArray(run.manifest.models) || run.manifest.models.some(model => !plannedModels.includes(model)))
      throw new Error('Run models must belong to the selected plan')
  }
  const pricing = prices === undefined ? undefined : pricingSnapshot(prices)
  const combined = (selected: typeof runs, plan: Record<string, unknown>) => {
    const rows = selected.flatMap(run => run.rows)
    const reports = selected.flatMap(run => run.reports)
    const seen = new Set<string>()
    for (const metadata of [...rows.map(row => record(record(row.response ?? {}).metadata ?? {})), ...reports.map(report => record(report.metadata))]) {
      if (metadata.sessionIDs === undefined) continue
      if (!Array.isArray(metadata.sessionIDs)) throw new Error('Invalid saved session list')
      for (const session of metadata.sessionIDs) {
        if (typeof session !== 'string' || seen.has(session)) throw new Error('Duplicate or invalid saved session')
        seen.add(session)
      }
    }
    return aggregate(plan, { results: { results: rows } },
      { models: modelSummaries(selected.flatMap(run => run.investigators), pricing) },
      { sessions: reports.length, executionErrors: reports.filter(report => report.status === 'execution-error').length,
        models: modelSummaries(reports.map(report => ({ provider: { label: record(report.metadata).model }, response: { metadata: report.metadata } })), pricing), judgments: reports }, prices)
  }
  const primary = runs.filter(run => run.manifest.retryOf === undefined)
  const data = combined(primary, manifest)
  if (data.repetitions !== null && data.models.some(model => model.tasks.some(task => task.attempts > data.repetitions!)))
    throw new Error('Selected attempts exceed the planned repetitions')
  const recoveries = runs.filter(run => run.manifest.retryOf !== undefined).map(run => combined([run], run.manifest))
  // A later active continuation takes precedence over a retained historical error.
  const state: LiveState = primary.some(run => run.state === 'running') ? 'running'
    : primary.some(run => run.state === 'stopped') ? 'stopped'
    : primary.length > 0 && primary.every(run => run.state === 'complete') && data.expectedAttempts !== null && data.attempts === data.expectedAttempts ? 'complete'
    : 'waiting'
  return { data, recoveries, state }
}

export type LiveData = ReturnType<typeof readLiveData>
