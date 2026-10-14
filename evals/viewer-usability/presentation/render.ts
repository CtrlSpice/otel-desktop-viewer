import type { Presentation } from './aggregate.ts'

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
    : `C$${value.toFixed(3)} est.${coverage === 'partial' ? ' (partial)' : ''}`
const cell = (title: string, value: string) =>
  `<div><dt>${escapeHtml(title)}</dt><dd>${escapeHtml(value)}</dd></div>`
const percent = (value: number | null) =>
  value === null ? 'Pending' : `${(value * 100).toFixed(1)}%`
const scores = (values: (number | null)[]) =>
  values.length ? values.map(value => value === null ? '—' : String(value)).join(' · ') : '—'

export function render(data: Presentation) {
  const maxTime = Math.max(
    1,
    ...data.models.map((model) => model.investigator.medianSeconds ?? 0)
  )
  const averages = data.models.map((model) =>
    model.investigator.apiCostCad !== null &&
    model.investigator.estimateCoverage === 'complete' &&
    model.investigator.attempts
      ? model.investigator.apiCostCad / model.investigator.attempts
      : null
  )
  const maxCost = Math.max(0.001, ...averages.map((value) => value ?? 0))
  const outcome = (value: Presentation['models'][number]['result']) =>
    `${value.pass} complete · ${value.partial} partial · ${value.fail} wrong · ${value.error} error${value.unknown ? ` · ${value.unknown} unknown` : ''}`
  const rows = data.models
    .map((model, index) => {
      const metrics = model.investigator,
        result = model.result
      const resultWidth = result.attempts
        ? (result.pass / result.attempts) * 100
        : 0
      const taskCards = model.tasks
        .map((task) => {
          const checks = task.checks,
            checkCount = checks.pass + checks.partial + checks.fail + checks.unknown
          return `<article class="task"><div class="task-title"><h3>${escapeHtml(task.task.replaceAll('-', ' '))}</h3><span class="task-count">${task.attempts}/${integer(data.repetitions)} repeats</span></div>
        <p class="task-result">${escapeHtml(outcome(task))}</p>
        <div class="repeat-strip" aria-hidden="true"><span class="pass" style="flex:${task.pass}"></span><span class="partial" style="flex:${task.partial};background:var(--accent)"></span><span class="fail" style="flex:${task.fail}"></span><span class="error" style="flex:${task.error + task.unknown}"></span></div>
        <dl>${cell('Automated scores', scores(task.scores))}${cell('Provisional mean', `${percent(task.provisionalMean)} · ${task.scoredAttempts}/${task.attempts} scored`)}
        ${cell('Human-signed scores', scores(task.signedScores))}${cell('Final three-run mean', percent(task.finalMean))}${cell('Signoff coverage', `${task.signedAttempts}/3 signed · ${task.signedFull} fully complete`)}
        ${task.claimReviewedAttempts ? cell('Human claim review', `${task.supportedClaims} supported · ${task.contradictedClaims} contradicted · ${task.unverifiedClaims} unverified`) : cell('Human claim review', 'Pending')}
        ${cell('Check verdicts', checkCount ? `${checks.pass} complete · ${checks.partial} partial · ${checks.fail} wrong · ${checks.unknown} unknown` : 'Unknown')}
        ${cell('Artifact check', checks.artifactPass + checks.artifactFail ? `${checks.artifactPass} pass · ${checks.artifactFail} fail · ${checks.artifactUnknown} unknown` : 'Unknown / not recorded')}
        ${cell('Median task time', seconds(task.medianSeconds))}${cell('Time coverage', `${task.timedAttempts}/${task.attempts} attempts`)}
        ${cell('Investigator cost', cost(task.apiCostCad, task.estimateCoverage))}${cell('Tool calls / failed', `${integer(task.toolCalls)} / ${integer(task.failedTools)}`)}
        ${cell('Input / output tokens', `${integer(task.inputTokens)} / ${integer(task.outputTokens)}`)}${cell('Cache read / write', `${integer(task.cacheReadTokens)} / ${integer(task.cacheWriteTokens)}`)}</dl></article>`
        })
        .join('')
      return `<details class="model"><summary><div class="model-name"><strong>${escapeHtml(model.name)}</strong><span>${escapeHtml(model.provider)} <span class="model-id">· ${escapeHtml(model.model)}</span></span></div>
       <div class="metric result"><span class="bar-track" aria-hidden="true"><span style="width:${resultWidth}%"></span></span><strong>${result.pass} complete · ${result.partial} partial</strong></div>
      <div class="metric duration"><span class="bar-track" aria-hidden="true"><span style="width:${((metrics.medianSeconds ?? 0) / maxTime) * 100}%"></span></span><strong>${seconds(metrics.medianSeconds)}</strong></div>
      <div class="metric cost"><span class="bar-track" aria-hidden="true"><span style="width:${((averages[index] ?? 0) / maxCost) * 100}%"></span></span><strong>${cost(averages[index], 'complete')}</strong></div>
      <span class="coverage">${result.attempts}/${data.tasksExpected !== null && data.repetitions !== null ? data.tasksExpected * data.repetitions : 'Unknown'} attempts</span><span class="chevron" aria-hidden="true">+</span></summary>
      <div class="expanded"><h2 class="sr-only">${escapeHtml(model.name)} task results</h2><div class="model-meta"><span>${escapeHtml(outcome(result))}</span><span>Investigator: ${integer(metrics.attempts)} sessions · ${escapeHtml(cost(metrics.apiCostCad, metrics.estimateCoverage))} total</span><span>${integer(metrics.toolCalls)} tool calls · ${integer(metrics.failedTools)} recorded failed tools</span></div><div class="tasks">${taskCards}</div></div></details>`
    })
    .join('')
  return `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><meta name="color-scheme" content="dark light"><title>Viewer investigation evals</title><style>
:root{color-scheme:dark;--bg:#141619;--panel:#1c1f24;--edge:#343940;--text:#f1f2f3;--muted:#afb7c1;--accent:#f9a66c;--pass:#88cabb;--fail:#e6a195;--track:#30353d;font-family:ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;background:var(--bg);color:var(--text)}
.sr-only{position:absolute;width:1px;height:1px;padding:0;margin:-1px;overflow:hidden;clip:rect(0,0,0,0);white-space:nowrap;border:0}
*{box-sizing:border-box}body{margin:0}main{max-width:1080px;margin:auto;padding:64px 28px}a{color:var(--accent);text-underline-offset:4px}h1{font-size:clamp(30px,5vw,46px);letter-spacing:-1.6px;margin:16px 0 12px;font-weight:650;line-height:1.15}.eyebrow{color:var(--accent);font:12px ui-monospace,monospace;letter-spacing:2px;text-transform:uppercase}.question{font-size:18px;color:var(--muted);margin:0 0 20px}.header-link{font-size:14px}.notice{margin:32px 0 18px;border-left:3px solid var(--accent);padding:12px 16px;background:var(--panel);font-size:13px;color:var(--muted);line-height:1.6}.notice strong{color:var(--accent)}.run-meta{display:flex;flex-wrap:wrap;gap:8px 24px;color:var(--muted);font-size:12px;margin-bottom:26px}button{font:inherit;cursor:pointer}.metrics{display:flex;gap:6px;border-bottom:1px solid var(--edge);padding-bottom:12px}.metrics button{border:1px solid transparent;background:none;color:var(--muted);padding:10px 15px;border-radius:6px;font-size:13px}.metrics button[aria-pressed="true"]{color:var(--text);background:var(--panel);border-color:var(--edge)}:focus-visible{outline:3px solid var(--accent);outline-offset:4px}.board-label{display:flex;justify-content:space-between;color:var(--muted);font:11px ui-monospace,monospace;text-transform:uppercase;letter-spacing:1px;margin:22px 0 8px}.model{border-bottom:1px solid var(--edge)}summary{list-style:none;display:grid;grid-template-columns:minmax(220px,1fr) minmax(240px,1fr) 110px 20px;gap:18px;align-items:center;padding:23px 0;cursor:pointer}summary::-webkit-details-marker{display:none}.model-name{display:flex;flex-direction:column;gap:5px;min-width:0}.model-name strong{font-size:16px}.model-name>span{font-size:11px;color:var(--muted);overflow-wrap:anywhere}.metric{display:flex;align-items:center;gap:18px}.metric strong{font:13px ui-monospace,monospace;white-space:nowrap;min-width:108px;text-align:right}.bar-track{height:7px;background:var(--track);border-radius:3px;flex:1;overflow:hidden}.bar-track>span{height:100%;background:var(--pass);display:block}.duration .bar-track>span{background:#a7b9e0}.cost .bar-track>span{background:var(--accent)}.metric.duration,.metric.cost{display:none}body[data-metric="duration"] .metric.result,body[data-metric="cost"] .metric.result{display:none}body[data-metric="duration"] .metric.duration,body[data-metric="cost"] .metric.cost{display:flex}.coverage{font-size:11px;text-align:right;color:var(--muted)}.chevron{font-size:23px;color:var(--muted);text-align:center}details[open] .chevron{transform:rotate(45deg)}.expanded{padding-bottom:24px}.model-meta{display:flex;flex-wrap:wrap;gap:10px 24px;font-size:12px;color:var(--muted);margin-bottom:18px}.tasks{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px}.task{background:var(--panel);border:1px solid var(--edge);border-radius:7px;padding:18px}.task-title{display:flex;gap:8px;justify-content:space-between;align-items:baseline}h3{font-size:14px;font-weight:600;margin:0;text-transform:capitalize}.task-count{font:11px ui-monospace,monospace;color:var(--muted);white-space:nowrap}.task-result{font:12px ui-monospace,monospace;margin:14px 0 10px}.repeat-strip{height:3px;display:flex;gap:3px}.repeat-strip span{min-width:0}.pass{background:var(--pass)}.fail{background:var(--fail)}.error{background:#aab3c0}dl{display:grid;grid-template-columns:1fr 1fr;gap:16px 12px;margin:18px 0 0}dt{font-size:11px;color:var(--muted);margin-bottom:5px}dd{font:11px ui-monospace,monospace;margin:0;line-height:1.5;overflow-wrap:anywhere}.footnote{font-size:12px;color:var(--muted);line-height:1.7;margin:18px 0}.method{border-top:1px solid var(--edge);margin-top:40px;padding-top:26px}.method h2{font-size:19px}.method p{font-size:13px;line-height:1.8;color:var(--muted);max-width:850px}.versions{font:11px ui-monospace,monospace;line-height:1.8;overflow-wrap:anywhere;color:var(--muted)}.grader{display:flex;flex-wrap:wrap;gap:10px 24px;font-size:12px;color:var(--muted)}
@media(max-width:700px){main{padding:32px 18px}.metrics{flex-wrap:wrap;gap:0}.metrics button{padding:9px 10px}summary{grid-template-columns:1fr 20px;gap:12px;padding:20px 0}.model-name{grid-column:1}.chevron{grid-column:2;grid-row:1}.metric{grid-column:1;grid-row:2}.coverage{grid-column:1;grid-row:3;text-align:left}.tasks{grid-template-columns:1fr}.board-label span:last-child{display:none}.model-id{display:none}.task{padding:16px}.model-meta{flex-direction:column;gap:7px}.metric strong{min-width:115px}}
@media(prefers-color-scheme:light){:root{color-scheme:light;--bg:#faf9f6;--panel:#fff;--edge:#d0d4d8;--text:#22252a;--muted:#555f6c;--accent:#98501b;--pass:#28796b;--fail:#ae4b3b;--track:#e6e8eb}}
</style></head><body data-metric="result"><main><header><div class="eyebrow">OpenTelemetry Desktop Viewer</div><h1>Eval results</h1></header>
<div class="notice"><strong>${data.synthetic ? 'Synthetic preview · not model results.' : data.provisional ? 'Provisional grades · human signoff pending.' : 'Recorded attempts signed off.'}</strong> Completion uses 0, 0.5 or 1. Only three signed-off attempts produce a final task mean. These results are not a calibrated model ranking.</div>
<div class="run-meta"><span>Run ${escapeHtml(data.date)}</span><span>${data.models.length} models · ${data.tasksObserved}/${integer(data.tasksExpected)} tasks · ${integer(data.repetitions)} repetitions</span><span>${data.attempts}/${integer(data.expectedAttempts)} recorded attempts</span></div>
<div class="metrics" role="group" aria-label="Model metric"><button type="button" data-view="result" aria-pressed="true">Recorded result</button><button type="button" data-view="duration" aria-pressed="false">Median task time</button><button type="button" data-view="cost" aria-pressed="false">Estimated cost / task</button></div>
<div class="board-label"><span>Model · expand for tasks</span><span id="metric-label" aria-live="polite">Automated complete / partial</span></div><section aria-label="Model results">${rows}</section>
${data.grader.sessions !== null ? `<div class="grader"><strong>Grading</strong><span>${integer(data.grader.sessions)} sessions${data.grader.executionErrors !== null ? ` · ${data.grader.executionErrors} execution errors` : ''}</span>${data.grader.models.map((model) => `<span>${escapeHtml(model.name)}${model.apiCostCad !== null ? ` · ${escapeHtml(cost(model.apiCostCad, model.estimateCoverage))}` : ''}</span>`).join('')}</div>` : ''}
${data.pricing ? `<p class="footnote">API-price estimates · CAD · FX ${escapeHtml(data.pricing.fxDate)}.</p>` : ''}
</main>
<script>const buttons=document.querySelectorAll('[data-view]');const labels={result:'Automated complete / partial',duration:'Median seconds / investigator session',cost:'Estimated CAD / investigator session'};for(const button of buttons)button.addEventListener('click',()=>{document.body.dataset.metric=button.dataset.view;for(const item of buttons)item.setAttribute('aria-pressed',String(item===button));document.getElementById('metric-label').textContent=labels[button.dataset.view];});</script></body></html>`
}
