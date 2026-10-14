import { modelSummaries } from '../model-summary.ts'
import { pricingSnapshot } from '../pricing.ts'

const object = (value: unknown): Record<string, unknown> =>
  value !== null && typeof value === 'object' && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {}
const array = (value: unknown): unknown[] => (Array.isArray(value) ? value : [])
const number = (value: unknown) =>
  typeof value === 'number' && Number.isFinite(value) && value >= 0
    ? value
    : null
const count = (value: unknown) =>
  Number.isSafeInteger(value) ? number(value) : null
const label = (value: unknown, fallback: string) =>
  typeof value === 'string' &&
  value.length <= 120 &&
  !/[\\/\x00-\x1f]/.test(value)
    ? value
    : fallback
const id = (value: unknown, model = false) => {
  if (
    typeof value !== 'string' ||
    !(
      model ? /^[a-zA-Z0-9_.:-]+\/[a-zA-Z0-9_.:-]+$/ : /^[a-zA-Z0-9_.:-]+$/
    ).test(value)
  )
    throw new Error('Invalid presentation identifier')
  return value
}
const hash = (value: unknown) =>
  typeof value === 'string' && /^[a-f0-9]{40}$/.test(value) ? value : 'Unknown'
const completionScore = (value: unknown) =>
  value === 0 || value === 0.5 || value === 1 ? value : null

// Supplied only by the validated human-review reader; never embed review records.
export type ReviewedAttempt = {
  model: string
  task: string
  sessionIDs: string[]
  score: number | null
  signedOff: boolean
  claimCounts?: { supported: number; contradicted: number; unverified: number } | null
}

// Reconstruct only fields needed by the existing usage/pricing calculator.
function metrics(value: unknown) {
  const source = object(value),
    usage = object(source.usage)
  return {
    elapsedSeconds: number(source.elapsedSeconds) ?? undefined,
    toolCalls: count(source.toolCalls) ?? undefined,
    failedToolCalls: count(source.failedToolCalls) ?? undefined,
    usage: Object.fromEntries(
      ['input', 'output', 'reasoning', 'cacheRead', 'cacheWrite'].map((key) => [
        key,
        count(usage[key]) ?? undefined,
      ])
    ),
    usageSteps: array(source.usageSteps).map((raw) => {
      const step = object(raw)
      return Object.fromEntries(
        ['input', 'output', 'reasoning', 'cacheRead', 'cacheWrite'].map(
          (key) => [key, count(step[key]) ?? undefined]
        )
      )
    }),
  }
}

export function aggregate(
  manifestValue: unknown,
  pilotValue: unknown,
  summaryValue: unknown,
  graderValue: unknown,
  pricingValue?: unknown,
  reviewed: ReviewedAttempt[] = []
) {
  const manifest = object(manifestValue),
    grader = object(graderValue)
  const pricing =
    pricingValue === undefined ? undefined : pricingSnapshot(pricingValue)
  const seenSessions = new Set<string>()
  const rows = array(object(object(pilotValue).results).results).map((raw) => {
    const row = object(raw),
      response = object(row.response),
      metadata = object(response.metadata)
    const grading = object(row.gradingResult)
    const reports = array(grading.componentResults).map((raw) =>
      object(object(raw).metadata)
    )
    const criteria = reports
      .flatMap((report) => array(report.criteria))
      .map(object)
    const artifact = reports
      .map((report) => object(report.artifact))
      .find((value) => typeof value.pass === 'boolean')
    const error =
      Boolean(response.error) ||
      row.failureReason === 2 ||
      Boolean(row.error && row.failureReason !== 1 && !row.gradingResult)
    const score = error ? null : completionScore(grading.score)
    const sessions = array(metadata.sessionIDs)
    for (const session of sessions) {
      if (typeof session !== 'string' || seenSessions.has(session))
        throw new Error('Invalid or duplicate investigator session in presentation')
      seenSessions.add(session)
    }
    const reviews = reviewed.filter(review =>
      review.model === (metadata.model ?? object(row.provider).label) &&
      review.task === object(row.vars).task_id &&
      sessions.length > 0 &&
      sessions.length === review.sessionIDs.length &&
      sessions.every(session => review.sessionIDs.includes(String(session)))
    )
    if (reviews.length > 1) throw new Error('Duplicate human review for one attempt')
    const human = reviews[0]
    const signedScore = !error && human?.signedOff ? completionScore(human.score) : null
    return {
      model: id(metadata.model ?? object(row.provider).label, true),
      task: id(object(row.vars).task_id),
      outcome: error
        ? 'error'
        : score === 0.5
          ? 'partial'
          : row.success === true
          ? 'pass'
          : row.success === false
            ? 'fail'
            : 'unknown',
      metadata: metrics(metadata),
      score,
      signedScore,
      claimCounts: human?.claimCounts ?? null,
      checks: {
        pass: criteria.filter((value) => value.verdict === 'Pass').length,
        fail: criteria.filter((value) => value.verdict === 'Fail').length,
        partial: criteria.filter((value) => value.verdict === 'Partial').length,
        unknown: criteria.filter(
          (value) => !['Pass', 'Partial', 'Fail'].includes(String(value.verdict))
        ).length,
        artifact: artifact
          ? artifact.pass === true
            ? 'pass'
            : 'fail'
          : 'unknown',
      },
      version: label(metadata.version, 'Unknown'),
    }
  })
  const taskIDs = [...new Set(rows.map((row) => row.task))]
  const declaredModels = array(manifest.models).map((value) => id(value, true))
  const summaries = array(object(summaryValue).models).map(object)
  const models = [
    ...new Set([
      ...declaredModels,
      ...rows.map((row) => row.model),
      ...summaries.map((value) => id(value.model, true)),
    ]),
  ]
  const repetitions = count(manifest.repetitions),
    tasksExpected = count(manifest.tasks)
  const summarise = (model: string, selected: typeof rows) => {
    const result = modelSummaries(
      selected.map((row) => ({
        provider: { label: model },
        response: { metadata: row.metadata },
      })),
      pricing
    )[0]
    return {
      attempts: selected.length,
      pass: selected.filter((row) => row.outcome === 'pass').length,
      fail: selected.filter((row) => row.outcome === 'fail').length,
      partial: selected.filter((row) => row.outcome === 'partial').length,
      scoredAttempts: selected.filter(row => row.score !== null).length,
      provisionalMean: selected.some(row => row.score !== null)
        ? selected.reduce((sum, row) => sum + (row.score ?? 0), 0) /
          selected.filter(row => row.score !== null).length
        : null,
      signedAttempts: selected.filter(row => row.signedScore !== null).length,
      signedFull: selected.filter(row => row.signedScore === 1).length,
      claimReviewedAttempts: selected.filter(row => row.claimCounts !== null).length,
      supportedClaims: selected.reduce((sum, row) => sum + (row.claimCounts?.supported ?? 0), 0),
      contradictedClaims: selected.reduce((sum, row) => sum + (row.claimCounts?.contradicted ?? 0), 0),
      unverifiedClaims: selected.reduce((sum, row) => sum + (row.claimCounts?.unverified ?? 0), 0),
      error: selected.filter((row) => row.outcome === 'error').length,
      unknown: selected.filter((row) => row.outcome === 'unknown').length,
      medianSeconds: result?.medianSeconds ?? null,
      timedAttempts: selected.filter(
        (row) => row.metadata.elapsedSeconds !== undefined
      ).length,
      toolCalls: result?.toolCalls ?? null,
      failedTools: result?.failedTools ?? null,
      inputTokens: result?.inputTokens ?? null,
      outputTokens: result?.outputTokens ?? null,
      cacheReadTokens: result?.cacheReadTokens ?? null,
      cacheWriteTokens: result?.cacheWriteTokens ?? null,
      apiCostCad: result?.apiCostCad ?? null,
      estimateCoverage: result?.estimateCoverage ?? 'unavailable',
    }
  }
  // Saved summaries account for sessions omitted by Promptfoo after grading errors.
  const savedMetrics = (value: Record<string, unknown>) => ({
    attempts: count(value.attempts),
    medianSeconds: number(value.medianSeconds),
    toolCalls: count(value.toolCalls),
    failedTools: count(value.failedTools),
    inputTokens: count(value.inputTokens),
    outputTokens: count(value.outputTokens),
    cacheReadTokens: count(value.cacheReadTokens),
    cacheWriteTokens: count(value.cacheWriteTokens),
    apiCostCad: number(value.apiCostCad),
    estimateCoverage: ['complete', 'partial', 'unavailable'].includes(
      String(value.estimateCoverage)
    )
      ? String(value.estimateCoverage)
      : 'unavailable',
  })
  const date =
    typeof manifest.startedAt === 'string' &&
    /^\d{4}-\d\d-\d\dT/.test(manifest.startedAt) &&
    Number.isFinite(Date.parse(manifest.startedAt))
      ? new Date(manifest.startedAt).toISOString()
      : 'Unknown'
  const dataset =
    typeof manifest.dataset === 'string'
      ? manifest.dataset.match(/(?:^|\/)testdata\/otlp\/([a-zA-Z0-9_-]+)$/)?.[1]
      : undefined
  return {
    date,
    synthetic: manifest.synthetic === true,
    provisional: rows.some(row => row.signedScore === null),
    repetitions,
    tasksExpected,
    tasksObserved: taskIDs.length,
    attempts: rows.length,
    expectedAttempts:
      repetitions !== null && tasksExpected !== null
        ? models.length * tasksExpected * repetitions
        : null,
    dataset: dataset ? `testdata/otlp/${dataset}` : 'Unknown',
    viewerCommit: hash(manifest.viewerSourceCommit),
    suiteCommit: hash(manifest.evalSourceCommit),
    cliVersions: [...new Set(rows.map((row) => row.version))],
    pricing: pricing
      ? {
          capturedAt: pricing.capturedAt.match(/^\d{4}-\d\d-\d\dT/)
            ? pricing.capturedAt.slice(0, 10)
            : 'Unknown',
          fxDate: pricing.exchange.date.match(/^\d{4}-\d\d-\d\d$/)
            ? pricing.exchange.date
            : 'Unknown',
          cadPerUsd: pricing.exchange.cadPerUsd,
        }
      : null,
    models: models.map((model) => {
      const selected = rows.filter((row) => row.model === model),
        calculated = summarise(model, selected)
      const saved = summaries.find((value) => value.model === model)
      return {
        model,
        name: label(
          pricing?.api.models[model]?.name ?? saved?.name,
          model.split('/')[1]
        ),
        provider: model.split('/')[0],
        result: calculated,
        investigator: saved
          ? savedMetrics(saved)
          : { ...savedMetrics(calculated), attempts: calculated.attempts },
        tasks: taskIDs.map((task) => {
          const attempts = selected.filter((row) => row.task === task)
          return {
            task,
            ...summarise(model, attempts),
            scores: attempts.map(row => row.score),
            signedScores: attempts.map(row => row.signedScore),
            finalMean: repetitions === 3 && attempts.length === 3 &&
              attempts.every(row => row.signedScore !== null)
              ? attempts.reduce((sum, row) => sum + row.signedScore!, 0) / 3
              : null,
            checks: {
              pass: attempts.reduce((sum, row) => sum + row.checks.pass, 0),
              fail: attempts.reduce((sum, row) => sum + row.checks.fail, 0),
              partial: attempts.reduce((sum, row) => sum + row.checks.partial, 0),
              unknown: attempts.reduce(
                (sum, row) => sum + row.checks.unknown,
                0
              ),
              artifactPass: attempts.filter(
                (row) => row.checks.artifact === 'pass'
              ).length,
              artifactFail: attempts.filter(
                (row) => row.checks.artifact === 'fail'
              ).length,
              artifactUnknown: attempts.filter(
                (row) => row.checks.artifact === 'unknown'
              ).length,
            },
          }
        }),
      }
    }),
    grader: {
      sessions: count(grader.sessions),
      executionErrors: count(grader.executionErrors),
      models: array(grader.models).map((raw) => {
        const value = object(raw)
        return {
          name: label(value.name, 'Separate grader'),
          ...savedMetrics(value),
        }
      }),
    },
  }
}

export type Presentation = ReturnType<typeof aggregate>
