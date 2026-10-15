import type { Presentation } from './aggregate.ts'

type Output = Presentation['grader']['outputs'][number]

const total = (values: (number | null)[]) => {
  if (values.some(value => value === null)) return null
  const sum = values.reduce<number>((sum, value) => sum + (value ?? 0), 0)
  return Number.isFinite(sum) ? sum : null
}

const countTotal = (values: (number | null)[]) => {
  const sum = total(values)
  return sum !== null && Number.isSafeInteger(sum) ? sum : null
}

function gradingMetrics(grades: Output[]) {
  const knownCosts = grades.map(grade => grade.costCad).filter(value => value !== null)
  const complete = grades.every(grade => grade.costCad !== null && grade.estimateCoverage === 'complete')
  return {
    totalSeconds: total(grades.map(grade => grade.seconds)),
    inputTokens: countTotal(grades.map(grade => grade.inputTokens)),
    outputTokens: countTotal(grades.map(grade => grade.outputTokens)),
    costCad: knownCosts.length ? total(knownCosts) : null,
    estimateCoverage: complete ? 'complete' : knownCosts.length ? 'partial' : 'unavailable',
  }
}

export function gradingScoreTotals(questions: { grades: Output[] }[], columns: number) {
  return Array.from({ length: columns }, (_, index) => {
    const grades = questions.flatMap(question => question.grades[index] ? [question.grades[index]] : [])
    const scored = grades.filter(grade => !grade.executionError && grade.score !== null)
    return {
      earned: scored.length ? scored.reduce((sum, grade) => sum + (grade.score ?? 0), 0) : null,
      graded: scored.length,
      errors: grades.filter(grade => grade.executionError).length,
    }
  })
}

export function groupGradingOutputs(outputs: Output[], modelOrder: string[], taskOrder: string[]) {
  const models = [...new Set([...modelOrder, ...outputs.map(output => output.investigatorModel)])]
  const tasks = [...new Set([...taskOrder, ...outputs.map(output => output.task)])]
  return models.flatMap(model => {
    const selected = outputs.filter(output => output.investigatorModel === model)
    if (!selected.length) return []
    return [{ model, totals: gradingMetrics(selected), questions: tasks.flatMap(task => {
      const grades = selected.filter(output => output.task === task)
      if (!grades.length) return []
      return [{ task, grades, ...gradingMetrics(grades) }]
    }) }]
  })
}
