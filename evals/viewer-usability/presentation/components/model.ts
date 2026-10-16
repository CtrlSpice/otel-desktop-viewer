import tasks from '../../tasks.json'
import type { Presentation } from '../aggregate.ts'

export type { Presentation }
export type Model = Presentation['models'][number]
export type Task = Model['tasks'][number]
export type Grader = Presentation['grader']['models'][number]
export type Timing = Pick<
  Model['investigator'],
  'totalSeconds' | 'medianSeconds' | 'p95Seconds' | 'maxSeconds'
>
export {
  integer,
  seconds,
  cost,
  percent,
  runResult,
  gradeLabel,
} from '../format.ts'
export {
  groupGradingOutputs,
  gradingScoreTotals,
  gradingStatus,
} from '../grading-output.ts'

export const taskName = (id: string) =>
  tasks.find(task => task.id === id)?.goal ?? id.replaceAll('-', ' ')
export const taskOrder = tasks.map(task => task.id)

export function outcomeBreakdown(value: Model['result']) {
  return [
    { status: 'complete', count: value.displayPass, label: 'complete' },
    { status: 'partial', count: value.displayPartial, label: 'partial' },
    {
      status: 'no-credit',
      count: value.displayFail,
      label: 'incorrect or missing',
    },
    {
      status: 'harness-error',
      count: value.harnessError,
      label: value.harnessError === 1 ? 'harness error' : 'harness errors',
    },
    {
      status: 'execution-error',
      count: value.error - value.harnessError,
      label:
        value.error - value.harnessError === 1
          ? 'execution error'
          : 'execution errors',
    },
    {
      status: 'unscored',
      count:
        value.attempts -
        value.displayPass -
        value.displayPartial -
        value.displayFail -
        value.error,
      label: 'unscored',
    },
  ]
    .filter(part => part.count > 0)
    .map(part => ({
      ...part,
      text: `${part.count.toLocaleString('en-CA')} ${part.label} · ${((part.count / value.attempts) * 100).toLocaleString('en-CA', { maximumFractionDigits: 1 })}%`,
    }))
}

export function averageCost(value: Model['investigator'] | Grader) {
  return value.apiCostCad !== null &&
    value.estimateCoverage === 'complete' &&
    value.attempts
    ? value.apiCostCad / value.attempts
    : null
}
