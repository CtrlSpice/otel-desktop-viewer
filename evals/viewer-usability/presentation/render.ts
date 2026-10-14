import type { Presentation } from './aggregate.ts'
import { caret, warningIcon, meanIcon, timeIcon, p95Icon, costIcon } from './viewer-components.ts'
import tasks from '../tasks.json' with { type: 'json' }

export const escapeHtml = (value: unknown) =>
  String(value).replace(
    /[&<>"']/g,
    (character) =>
      ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[
        character
      ]!
  )
const integer = (value: number | null) =>
  value === null ? 'Unknown' : value.toLocaleString('en-CA')
const seconds = (value: number | null) =>
  value === null ? 'Unknown' : `${value.toFixed(1)} s`
const cost = (value: number | null, coverage: string) =>
  value === null
    ? 'Unknown'
    : `C$${value.toFixed(3)}${coverage === 'partial' ? ' (partial)' : ''}`
const cell = (title: string, value: string, emphasis = false) =>
  `<div${emphasis ? ' class="mean"' : ''}><dt>${escapeHtml(title)}</dt><dd>${escapeHtml(value)}</dd></div>`
const percent = (value: number | null) =>
  value === null ? 'Unscored' : `${(value * 100).toFixed(1)}%`
const scores = (values: (number | null)[]) =>
  values.length ? values.map(value => value === null ? '—' : String(value)).join(' · ') : '—'
const runResult = (value: number) =>
  value === 1 ? 'Complete (1)' : value === 0.5 ? 'Partial (0.5)' : 'No credit (0)'
const resultBadge = (value: number, includeScore = true, signedOff = false) => {
  const status = value === 1 ? 'complete' : value === 0.5 ? 'partial' : 'no-credit'
  const label = includeScore ? runResult(value) : value === 1 ? 'Complete' : value === 0.5 ? 'Partial' : 'No credit'
  return `<span class="badge badge-soft result-badge result-${status}">${escapeHtml(label)}${signedOff ? ' · signed' : ''}</span>`
}
const resultCell = (title: string, value: number) =>
  `<div><dt>${escapeHtml(title)}</dt><dd>${resultBadge(value)}</dd></div>`
const taskName = (id: string) => tasks.find(task => task.id === id)?.goal ?? id.replaceAll('-', ' ')

export function render(data: Presentation, fontFaces = '', componentStyles = '') {
  const maxTime = Math.max(
    1,
    ...data.models.map((model) => model.investigator.medianSeconds ?? 0),
    ...data.grader.models.map(model => model.medianSeconds ?? 0)
  )
  const maxP95 = Math.max(1, ...data.models.map(model => model.investigator.p95Seconds ?? 0),
    ...data.grader.models.map(model => model.p95Seconds ?? 0))
  const averages = data.models.map((model) =>
    model.investigator.apiCostCad !== null &&
    model.investigator.estimateCoverage === 'complete' &&
    model.investigator.attempts
      ? model.investigator.apiCostCad / model.investigator.attempts
      : null
  )
  const graderAverages = data.grader.models.map(model => model.apiCostCad !== null && model.estimateCoverage === 'complete' && model.attempts ? model.apiCostCad / model.attempts : null)
  const maxCost = Math.max(0.001, ...averages.map((value) => value ?? 0), ...graderAverages.map(value => value ?? 0))
  const outcome = (value: Presentation['models'][number]['result']) =>
    [[value.displayPass, 'complete'], [value.displayPartial, 'partial'], [value.displayFail, 'no credit'],
      [value.error, 'error'], [value.unknown, 'unknown']]
      .filter(([count]) => Number(count) > 0)
      .map(([count, label]) => `${count} ${label}`).join(' · ') || 'No results'
  const rows = data.models
    .map((model, index) => {
       const metrics = model.investigator,
         result = model.result
        const completionMean = result.displayMean
       const resultWidth = (completionMean ?? 0) * 100
      const taskCards = model.tasks
        .map((task) => {
           const checks = task.checks
           const singleScore = task.signedScores[0] ?? task.scores[0]
            return `<article class="task card bg-base-200"><div class="task-title pane-header"><h3>${escapeHtml(taskName(task.task))}</h3>${data.repetitions !== 1 ? `<span class="task-count">${task.attempts}/${integer(data.repetitions)} runs</span>` : ''}</div>
         <div class="task-group result-group">${task.attempts > 1 ? `<p class="task-result">${escapeHtml(outcome(task))}</p>
         <div class="repeat-strip" aria-hidden="true"><span class="pass" style="flex:${task.displayPass}"></span><span class="partial" style="flex:${task.displayPartial};background:var(--accent)"></span><span class="fail" style="flex:${task.displayFail}"></span><span class="error" style="flex:${task.error + task.unknown}"></span></div>` : ''}
          <dl>${task.attempts === 1 ? singleScore !== null && singleScore !== undefined ? resultCell('Result', singleScore) : cell('Result', task.error ? 'Execution error' : 'Unscored') : task.displayMean !== null ? cell('Mean', percent(task.displayMean), true) + (task.scoredAttempts !== task.attempts ? cell('Scored runs', `${task.scoredAttempts}/${task.attempts}`) : '') : cell('Result', task.error ? 'Execution error' : 'Unscored')}
        ${task.signedAttempts ? cell('Human signoff', task.attempts === 1 ? 'Signed off' : `${task.signedAttempts}/${task.attempts} runs signed`) : ''}
         ${task.attempts === 1 && task.signedAttempts && task.scores[0] !== singleScore && task.scores[0] !== null ? resultCell('Automated result', task.scores[0]!) : ''}
        ${task.claimReviewedAttempts ? cell('Human claim review', `${task.supportedClaims} supported · ${task.contradictedClaims} contradicted · ${task.unverifiedClaims} unverified`) : ''}
         ${checks.artifactPass + checks.artifactFail ? cell('Artifact check', checks.artifactPass + checks.artifactFail === 1 ? (checks.artifactPass ? 'Passed' : 'Failed') : [[checks.artifactPass, 'passed'], [checks.artifactFail, 'failed']].filter(([count]) => Number(count) > 0).map(([count, label]) => `${count} ${label}`).join(' · ')) : ''}
           </dl>${task.attempts > 1 ? `<table class="table run-results"><thead><tr><th scope="col">Run</th><th scope="col">Result</th><th scope="col">Score</th></tr></thead><tbody>${task.runResults.map(run => `<tr><th scope="row">Run ${run.run}</th><td>${run.status === 'error' ? 'Execution error' : run.score === null ? 'Unscored' : resultBadge(run.score, false, run.signedOff)}</td><td>${run.score === null ? '—' : run.score}</td></tr>`).join('')}</tbody></table>` : ''}</div>
          <div class="task-group"><dl>${cell('Median time', seconds(task.medianSeconds))}${task.attempts > 1 ? cell('P95 time', seconds(task.p95Seconds)) : ''}
         ${task.timedAttempts !== task.attempts ? cell('Time coverage', `${task.timedAttempts}/${task.attempts} attempts`) : ''}
         ${cell('Tool calls', integer(task.toolCalls))}${task.failedTools ? cell('Failed tools', integer(task.failedTools)) : ''}</dl></div>
         <div class="task-group"><dl>${cell('Input tokens', integer(task.inputTokens))}${cell('Output tokens', integer(task.outputTokens))}
         ${cell('Cache read', integer(task.cacheReadTokens))}${task.cacheWriteTokens ? cell('Cache write', integer(task.cacheWriteTokens)) : ''}
           ${cell('Cost (est., CAD)', cost(task.apiCostCad, task.estimateCoverage))}</dl></div></article>`
        })
        .join('')
      return `<details class="model"><summary><div class="model-name"><strong>${escapeHtml(model.name)}</strong><span>${escapeHtml(model.provider)} <span class="model-id">· ${escapeHtml(model.model)}</span></span></div>
        <div class="metric result"><span class="bar-track" aria-hidden="true"><span style="width:${resultWidth}%"></span></span><strong>${percent(completionMean)}</strong></div>
      <div class="metric duration"><span class="bar-track" aria-hidden="true"><span style="width:${((metrics.medianSeconds ?? 0) / maxTime) * 100}%"></span></span><strong>${seconds(metrics.medianSeconds)}</strong></div>
      <div class="metric p95"><span class="bar-track" aria-hidden="true"><span style="width:${((metrics.p95Seconds ?? 0) / maxP95) * 100}%"></span></span><strong>${seconds(metrics.p95Seconds)}</strong></div>
      <div class="metric cost"><span class="bar-track" aria-hidden="true"><span style="width:${((averages[index] ?? 0) / maxCost) * 100}%"></span></span><strong>${cost(averages[index], 'complete')}</strong></div>
       <span class="coverage">${result.attempts}/${data.tasksExpected !== null && data.repetitions !== null ? data.tasksExpected * data.repetitions : 'Unknown'} runs</span><span class="chevron" aria-hidden="true">${caret}</span></summary>
       <div class="expanded"><h2 class="sr-only">${escapeHtml(model.name)} task results</h2><div class="model-meta"><span>${escapeHtml(outcome(result))}</span><span>Investigator: ${integer(metrics.attempts)} sessions · total cost (est., CAD) ${escapeHtml(cost(metrics.apiCostCad, metrics.estimateCoverage))}</span><span>${integer(metrics.toolCalls)} tool calls · ${integer(metrics.failedTools)} recorded failed tools</span></div><div class="tasks">${taskCards}</div></div></details>`
    })
    .join('')
  const graderRows = data.grader.models.map((model, index) => {
    const outputs = data.grader.outputs.filter(output => output.graderModel === model.model)
    return `<details class="model grader-row"><summary><div class="model-name"><strong>Grader</strong><span>${escapeHtml(model.name)}</span></div>
      <div class="metric result"><strong>${integer(model.attempts)} grades</strong></div>
      <div class="metric duration"><span class="bar-track" aria-hidden="true"><span style="width:${((model.medianSeconds ?? 0) / maxTime) * 100}%"></span></span><strong>${seconds(model.medianSeconds)}</strong></div>
      <div class="metric p95"><span class="bar-track" aria-hidden="true"><span style="width:${((model.p95Seconds ?? 0) / maxP95) * 100}%"></span></span><strong>${seconds(model.p95Seconds)}</strong></div>
      <div class="metric cost"><span class="bar-track" aria-hidden="true"><span style="width:${((graderAverages[index] ?? 0) / maxCost) * 100}%"></span></span><strong>${cost(graderAverages[index], 'complete')}</strong></div>
      <span class="coverage">${integer(model.attempts)} grades</span><span class="chevron" aria-hidden="true">${caret}</span></summary>
      <div class="expanded"><dl class="grader-metrics">${cell('Sessions', integer(model.attempts))}${cell('Median time', seconds(model.medianSeconds))}${cell('P95 time', seconds(model.p95Seconds))}${cell('Input tokens', integer(model.inputTokens))}${cell('Output tokens', integer(model.outputTokens))}${cell('Total cost (est., CAD)', cost(model.apiCostCad, model.estimateCoverage))}${data.grader.executionErrors ? cell('Execution errors', String(data.grader.executionErrors)) : ''}</dl>
      ${outputs.length ? `<div class="grading-output" role="region" aria-label="Grading outputs" tabindex="0"><table class="table"><caption>Grading outputs</caption><thead><tr><th scope="col">Investigator</th><th scope="col">Task</th><th scope="col">Output</th><th scope="col">Time</th><th scope="col">Cost (est., CAD)</th><th scope="col">Input tokens</th><th scope="col">Output tokens</th></tr></thead><tbody>${outputs.map(output => `<tr><th scope="row">${escapeHtml(data.models.find(investigator => investigator.model === output.investigatorModel)?.name ?? output.investigatorModel)}</th><td>${escapeHtml(taskName(output.task))}</td><td>${output.executionError ? 'Execution error' : output.score === null ? 'Unscored' : resultBadge(output.score)}</td><td>${seconds(output.seconds)}</td><td>${cost(output.costCad, output.estimateCoverage)}</td><td>${integer(output.inputTokens)}</td><td>${integer(output.outputTokens)}</td></tr>`).join('')}</tbody></table></div>` : ''}</div></details>`
  }).join('')
  return `<!doctype html>
<html lang="en" data-theme="rose-pine"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><meta name="color-scheme" content="dark light"><title>Viewer investigation evals</title><script>const theme=matchMedia('(prefers-color-scheme: light)');const applyTheme=()=>document.documentElement.dataset.theme=theme.matches?'rose-pine-dawn':'rose-pine';applyTheme();theme.addEventListener('change',applyTheme);</script><style>
${componentStyles}
${fontFaces}
:root{color-scheme:dark;--bg:#191724;--panel:#1f1d2e;--edge:#6e6a86;--text:#e0def4;--muted:#908caa;--accent:#c4a7e7;--pass:#31748f;--fail:#eb6f92;--track:#26233a;font-family:ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;background:var(--bg);color:var(--text)}
.sr-only{position:absolute;width:1px;height:1px;padding:0;margin:-1px;overflow:hidden;clip:rect(0,0,0,0);white-space:nowrap;border:0}
*{box-sizing:border-box}body{margin:0}main{max-width:1080px;margin:auto;padding:64px 28px}a{color:var(--accent);text-underline-offset:4px}h1{font-size:clamp(30px,5vw,46px);letter-spacing:-1.6px;margin:16px 0 12px;font-weight:650;line-height:1.15}.eyebrow{color:var(--accent);font:12px ui-monospace,monospace;letter-spacing:2px;text-transform:uppercase}.question{font-size:18px;color:var(--muted);margin:0 0 20px}.header-link{font-size:14px}.notice{margin:32px 0 18px;border-left:3px solid var(--accent);padding:12px 16px;background:var(--panel);font-size:13px;color:var(--muted);line-height:1.6}.notice strong{color:var(--accent)}.run-meta{display:flex;flex-wrap:wrap;gap:8px 24px;color:var(--muted);font-size:12px;margin-bottom:26px}button{font:inherit;cursor:pointer}.metrics{display:flex;gap:6px;border-bottom:1px solid var(--edge);padding-bottom:12px}.metrics button{border:1px solid transparent;background:none;color:var(--muted);padding:10px 15px;border-radius:6px;font-size:13px}.metrics button[aria-pressed="true"]{color:var(--text);background:var(--panel);border-color:var(--edge)}:focus-visible{outline:3px solid var(--accent);outline-offset:4px}.board-label{display:flex;justify-content:space-between;color:var(--muted);font:11px ui-monospace,monospace;text-transform:uppercase;letter-spacing:1px;margin:22px 0 8px}.model{border-bottom:1px solid var(--edge)}summary{list-style:none;display:grid;grid-template-columns:minmax(220px,1fr) minmax(240px,1fr) 110px 20px;gap:18px;align-items:center;padding:23px 0;cursor:pointer}summary::-webkit-details-marker{display:none}.model-name{display:flex;flex-direction:column;gap:5px;min-width:0}.model-name strong{font-size:16px}.model-name>span{font-size:11px;color:var(--muted);overflow-wrap:anywhere}.metric{display:flex;align-items:center;gap:18px}.metric strong{font:13px ui-monospace,monospace;white-space:nowrap;min-width:108px;text-align:right}.bar-track{height:7px;background:var(--track);border-radius:3px;flex:1;overflow:hidden}.bar-track>span{height:100%;background:var(--pass);display:block}.duration .bar-track>span{background:#a7b9e0}.cost .bar-track>span{background:var(--accent)}.metric.duration,.metric.cost{display:none}body[data-metric="duration"] .metric.result,body[data-metric="cost"] .metric.result{display:none}body[data-metric="duration"] .metric.duration,body[data-metric="cost"] .metric.cost{display:flex}.coverage{font-size:11px;text-align:right;color:var(--muted)}.chevron{font-size:23px;color:var(--muted);text-align:center}details[open] .chevron{transform:rotate(45deg)}.expanded{padding-bottom:24px}.model-meta{display:flex;flex-wrap:wrap;gap:10px 24px;font-size:12px;color:var(--muted);margin-bottom:18px}.tasks{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px}.task{background:var(--panel);border:1px solid var(--edge);border-radius:7px;padding:18px}.task-title{display:flex;gap:8px;justify-content:space-between;align-items:baseline}h3{font-size:14px;font-weight:600;margin:0;text-transform:capitalize}.task-count{font:11px ui-monospace,monospace;color:var(--muted);white-space:nowrap}.task-result{font:12px ui-monospace,monospace;margin:14px 0 10px}.repeat-strip{height:3px;display:flex;gap:3px}.repeat-strip span{min-width:0}.pass{background:var(--pass)}.fail{background:var(--fail)}.error{background:#aab3c0}dl{display:grid;grid-template-columns:1fr 1fr;gap:16px 12px;margin:18px 0 0}dt{font-size:11px;color:var(--muted);margin-bottom:5px}dd{font:11px ui-monospace,monospace;margin:0;line-height:1.5;overflow-wrap:anywhere}.footnote{font-size:12px;color:var(--muted);line-height:1.7;margin:18px 0}.method{border-top:1px solid var(--edge);margin-top:40px;padding-top:26px}.method h2{font-size:19px}.method p{font-size:13px;line-height:1.8;color:var(--muted);max-width:850px}.versions{font:11px ui-monospace,monospace;line-height:1.8;overflow-wrap:anywhere;color:var(--muted)}.grader{display:flex;flex-wrap:wrap;gap:10px 24px;font-size:12px;color:var(--muted)}
@media(max-width:700px){main{padding:32px 18px}.metrics{flex-wrap:wrap;gap:0}.metrics button{padding:9px 10px}summary{grid-template-columns:1fr 20px;gap:12px;padding:20px 0}.model-name{grid-column:1}.chevron{grid-column:2;grid-row:1}.metric{grid-column:1;grid-row:2}.coverage{grid-column:1;grid-row:3;text-align:left}.tasks{grid-template-columns:1fr}.board-label span:last-child{display:none}.model-id{display:none}.task{padding:16px}.model-meta{flex-direction:column;gap:7px}.metric strong{min-width:115px}}
@media(prefers-color-scheme:light){:root{color-scheme:light;--bg:#f2e9e1;--panel:#f8f1ea;--edge:#9893a5;--text:#575279;--muted:#575279;--accent:#907aa9;--pass:#286983;--fail:#b4637a;--track:#fffff9}.eyebrow,.notice strong{color:var(--text)}}
 .task-group{margin-top:20px;padding-top:12px;border-top:1px solid var(--edge)}.task-group h4{margin:0 0 12px;font-size:13px;font-weight:600;color:var(--text)}
 .board-label{display:grid;grid-template-columns:minmax(220px,1fr) minmax(240px,1fr) 110px 20px;gap:18px}#metric-label{text-align:right}.board-label .runs-label{text-align:right}@media(max-width:700px){.board-label{display:none}}
  .metrics{gap:4px;align-items:flex-end}.metrics button{border:0;border-radius:0;background:transparent;box-shadow:none;color:var(--text);white-space:nowrap;transition:color 200ms ease-out,background-color 200ms ease-out,box-shadow 200ms ease-out}.metrics button:hover{background:color-mix(in oklab,var(--text) 10%,var(--bg))}.metrics button[aria-pressed=true]{color:var(--text);background:transparent;box-shadow:inset 0 -2px var(--accent)}.metrics button:focus-visible{color:var(--text);outline:none;background:transparent;box-shadow:inset 0 -3px var(--accent)}summary:focus-visible{outline:1px solid var(--accent);outline-offset:1px;border-radius:8px}@media(prefers-color-scheme:light){.metrics button{color:var(--text)}}@media(forced-colors:active){.metrics button:focus-visible{outline:2px solid Highlight;outline-offset:-2px}}
 .grading-summary{margin-top:40px;padding-top:24px;border-top:1px solid var(--edge)}.grading-summary h2{margin:0 0 20px;font-size:16px}.grading-summary .grader{display:grid;grid-template-columns:repeat(auto-fit,minmax(160px,1fr));gap:24px;margin:0;padding:0;border:0}.grading-summary dt{font-size:12px;color:var(--muted)}.grading-summary dd{margin:8px 0 0;font-size:16px}.grading-summary .footnote{margin-top:24px}
 .tasks{gap:20px}.task{padding:24px;line-height:1.5}.task h3{font-size:18px;line-height:1.4}.task-group{margin-top:24px;padding-top:20px}.task-group h4{font-size:16px;margin-bottom:16px}.task dl{gap:20px 24px}.task dt{font-size:14px;line-height:1.5}.task dd{margin-top:6px;font-family:inherit;font-size:18px;line-height:1.4;letter-spacing:normal;font-variant-numeric:tabular-nums;color:var(--text)}.task .mean dd{font-size:28px;font-weight:600}.task-result{font-size:14px;line-height:1.5}.task-count{font-size:13px}@media(max-width:700px){.task{padding:20px}.task dl{gap:18px}.task dd{font-size:17px}}
 :root{--warning:#f6c177}.notice-warning{border-left-color:var(--warning);background:color-mix(in oklab,var(--warning) 10%,var(--bg))}.notice-warning strong{color:var(--warning)}@media(prefers-color-scheme:light){:root{--warning:#ea9d34}.notice-warning strong{color:var(--text)}}
 .task-group{border:0;padding-top:0;margin-top:28px}.task dl{margin:0}.task-group:first-of-type{margin-top:20px}
 .metrics,.model,.task,.grading-summary{border:0}
 .model>summary{border-bottom:1px solid var(--edge)}
 .metrics button{transition:background-color 200ms ease-out,box-shadow 200ms ease-out}
 :root{--font-label:14px;--font-body:16px;--font-title:24px;font-size:var(--font-body)}h1{font-size:var(--font-title);line-height:1.3}.eyebrow,.run-meta,.board-label,.model-name>span,.coverage,.model-meta,.task-count,.task dt,.grading-summary dt,.footnote,.metrics button,.notice{font-family:inherit;font-size:var(--font-label);line-height:1.5;letter-spacing:normal}.model-name strong,.metric strong,.task h3,.task dd,.task .mean dd,.task-result,.grading-summary h2,.grading-summary dd{font-family:inherit;font-size:var(--font-body);line-height:1.5;letter-spacing:normal;font-variant-numeric:tabular-nums}
 :root{font-family:"Atkinson Hyperlegible Next",system-ui,"Segoe UI",sans-serif}
 .task .result-group dd:not(.mean dd){font-size:var(--font-label);font-weight:400}.result-group{margin-top:20px}
 .result-complete{--result-colour:var(--color-info,#9ccfd8)}.result-partial{--result-colour:var(--color-warning,#f6c177)}.result-no-credit{--result-colour:var(--color-error,#eb6f92)}
 .result-badge{display:inline-flex;align-items:center;gap:6px;height:auto;min-height:0;padding:2px 8px;border:0;border-radius:6px;background:color-mix(in oklab,var(--result-colour) 12%,var(--panel));color:var(--text);font-family:inherit;font-size:var(--font-label);font-weight:400;line-height:1.5;white-space:nowrap}.result-badge::before{content:"";width:6px;height:6px;flex:none;border-radius:50%;background:var(--result-colour)}
 .run-results thead th,.grading-output thead th{color:var(--text)}
 @media(prefers-color-scheme:light){.result-complete{--result-colour:var(--color-info,#56949f)}.result-partial{--result-colour:var(--color-warning,#ea9d34)}.result-no-credit{--result-colour:var(--color-error,#b4637a)}}
  .chevron{transform:none!important;display:flex;align-items:center;justify-content:center}.chevron::before,.chevron::after{content:none!important}.model[open]>summary .chevron svg{transform:rotate(180deg)}.model>summary{border:0}.model{border-bottom:1px solid var(--edge)}.run-results{width:100%;margin-top:20px;font-size:var(--font-label)}.run-results th,.run-results td{padding:8px 0;text-align:left;font-family:inherit;font-size:var(--font-label);white-space:normal}.notice{display:flex;align-items:center;gap:12px}
 .task h3{text-transform:none}
 .task-title{background:var(--color-base-300,var(--track));margin:-24px -24px 0;padding:16px 24px;border-radius:8px 8px 0 0;align-items:flex-start}.task-title h3{font-weight:700;margin:0;color:var(--text)}@media(max-width:700px){.task-title{margin:-20px -20px 0;padding:14px 20px}}
 .task-title{height:auto;white-space:normal}.task dl{display:grid;grid-template-columns:1fr;gap:12px}.task dl>div{display:grid;grid-template-columns:minmax(0,1fr) minmax(0,1fr);gap:16px;align-items:baseline}.task dd{margin:0;text-align:right;overflow-wrap:anywhere}
 .metrics{display:grid;grid-template-columns:repeat(4,max-content);gap:4px;background:transparent;padding:4px;border-radius:8px}.metrics button{display:flex;align-items:center;gap:8px;min-width:0}.metrics svg{flex-shrink:0}.model .metric{display:none}body[data-metric=result] .model .metric.result,body[data-metric=duration] .model .metric.duration,body[data-metric=p95] .model .metric.p95,body[data-metric=cost] .model .metric.cost{display:flex}.grading-output{overflow-x:auto;max-width:100%;margin-top:24px}.grading-output table{min-width:900px;width:100%;font-size:var(--font-label)}.grading-output caption{text-align:left;font-size:var(--font-body);font-weight:600;margin-bottom:16px}.grading-output th,.grading-output td{padding:12px;text-align:left;vertical-align:top;white-space:normal}.grader-metrics{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:24px;margin:20px 0}.grader-metrics dt{font-size:var(--font-label);color:var(--muted)}.grader-metrics dd{margin:8px 0 0;font-size:var(--font-body)}@media(max-width:700px){.metrics{grid-template-columns:repeat(2,minmax(0,1fr))}.metrics button{justify-content:center}.grader-metrics{grid-template-columns:repeat(2,minmax(0,1fr))}}
 ${componentStyles ? ':root{--bg:var(--color-base-100);--panel:var(--color-base-200);--edge:var(--color-neutral);--text:var(--color-base-content);--muted:var(--color-subtle);--accent:var(--color-primary);--warning:var(--color-warning);--pass:var(--color-success);--fail:var(--color-error);--track:var(--color-base-300)}@media(prefers-color-scheme:light){:root{--muted:var(--color-base-content)}}' : ''}
 </style></head><body data-metric="result"><main><header><div class="eyebrow">OpenTelemetry Desktop Viewer</div><h1>Eval results</h1></header>
<div class="notice alert${data.provisional || data.synthetic ? ' alert-warning notice-warning' : ' alert-info'}">${data.provisional || data.synthetic ? warningIcon : ''}<strong>${data.synthetic ? 'Synthetic preview · not model results.' : data.provisional ? 'Provisional · awaiting human signoff.' : 'Recorded attempts signed off.'}</strong></div>
<div class="run-meta"><span>Run ${escapeHtml(data.date)}</span><span>${data.models.length} models · ${data.tasksObserved}/${integer(data.tasksExpected)} tasks · ${integer(data.repetitions)} repetitions</span><span>${data.attempts}/${integer(data.expectedAttempts)} recorded attempts</span></div>
<div class="metrics pane-header tabs" role="group" aria-label="Model metric"><button type="button" class="tab pane-header__tab tab-active" data-view="result" aria-pressed="true">${meanIcon}Mean</button><button type="button" class="tab pane-header__tab" data-view="duration" aria-pressed="false">${timeIcon}Median time</button><button type="button" class="tab pane-header__tab" data-view="p95" aria-pressed="false">${p95Icon}P95 time</button><button type="button" class="tab pane-header__tab" data-view="cost" aria-pressed="false">${costIcon}Cost / run (est., CAD)</button></div>
 <div class="board-label"><span>Model</span><span id="metric-label" aria-live="polite">Mean</span><span class="runs-label">Runs</span><span aria-hidden="true"></span></div><section aria-label="Model results">${rows}${graderRows}</section>
${data.pricing ? `<p class="footnote">API-price estimates · CAD · FX ${escapeHtml(data.pricing.fxDate)}.</p>` : ''}
</main>
 <script>const buttons=document.querySelectorAll('[data-view]');const labels={result:'Mean',duration:'Median time / run',p95:'P95 time / run',cost:'Cost / run (est., CAD)'};for(const button of buttons)button.addEventListener('click',()=>{document.body.dataset.metric=button.dataset.view;for(const item of buttons){item.setAttribute('aria-pressed',String(item===button));item.classList.toggle('tab-active',item===button);}document.getElementById('metric-label').textContent=labels[button.dataset.view];});</script></body></html>`.replace(/\n(?:[ \t]*\n)+/g, '\n')
}
