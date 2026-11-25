import { spawnSync } from 'node:child_process'
import { pathToFileURL } from 'node:url'

const documents = new Set(['/', '/llms.txt', '/openapi.json', '/openrpc.json'])
const inspections = new Set(`rpc.discover query getStats getServiceCount getServiceLabels discoverTelemetry
searchTraceSummaries getTraceView getTraceOverview getSpan getTraceSpanCount searchLogSummaries searchLogPage
getLogErrorNavigation getTraceLogSummaries getLog searchMetricSummaries getMetric getMetricSeries getMetricSeriesOrder
getMetricView getMetricAggregateView getTraceAttributeDefinitions getLogAttributeDefinitions getMetricAttributeDefinitions
getFieldValueCompletions searchAttributeMatches getTraceAttributeDefinitionsByTraceID`.split(/\s+/))

export function checkCurlCommand(command, endpoint) {
  if (!/\bcurl\b/.test(command)) return
  const tokens = []
  const word = /[ \t]*(?:'([^']*)'|"([^"`$\\]*)"|([a-zA-Z0-9_./:@%+=,-]+))(?=[ \t]|$)/y
  while (word.lastIndex < command.trimEnd().length) {
    const token = word.exec(command)
    if (!token) throw Error('Run curl as a separate command with literal arguments')
    tokens.push(token[1] ?? token[2] ?? token[3])
  }
  if (tokens.shift() !== 'curl') throw Error('Use curl directly against the assigned viewer')
  viewerRequest(tokens, endpoint)
}

// Rebuild the request from validated arguments. No arbitrary curl options,
// files, proxy settings, redirects or multiple transfers reach the executable.
export function viewerRequest(args, endpoint) {
  const origin = new URL(endpoint)
  if (origin.protocol !== 'http:' || origin.hostname !== '127.0.0.1' || !origin.port || origin.href !== origin.origin + '/')
    throw Error('Expected the assigned loopback viewer')
  const flags = []
  let url, method, body
  for (let i = 0; i < args.length; i++) {
    const a = args[i]
    if (['--silent','--show-error','--fail','--include'].includes(a)) flags.push(a)
    else if (a === '--head') method = 'HEAD'
    else if (/^-[fsSiI]+$/.test(a)) {
      if (a.includes('I')) method = 'HEAD'
      flags.push(a.replaceAll('I',''))
    } else if ((a === '-m' || a === '--max-time') && /^\d+(\.\d+)?$/.test(args[i+1])) flags.push('--max-time',args[++i])
    else if (a === '-X' || a === '--request') method = args[++i]
    else if (a === '-H' || a === '--header') {
      if (!/^content-type:\s*application\/json$/i.test(args[++i])) throw Error('Expected JSON header')
    } else if (['-d','--data','--data-raw','--json','--data-binary'].includes(a) && body === undefined) body = args[++i]
    else if (url === undefined && (a === origin.origin || a.startsWith(origin.origin + '/'))) url = a
    else throw Error('Only read-only requests to the assigned viewer are allowed')
  }
  if (!url) throw Error('Missing viewer URL')
  const route = url.slice(origin.origin.length) || '/'
  method ??= body === undefined ? 'GET' : 'POST'
  const safe = ['-q','--globoff','--noproxy','*', ...flags.filter(a=>a!=='-')]
  if (documents.has(route) && ['GET','HEAD'].includes(method) && body === undefined)
    return [...safe,...(method==='HEAD'?['--head']:[]),'--',origin.origin+route]
  if (route === '/rpc' && method === 'POST' && body?.trimStart().startsWith('{')) {
    const rpc = JSON.parse(body)
    if (rpc.jsonrpc === '2.0' && inspections.has(rpc.method) && Object.hasOwn(rpc,'id') && Object.keys(rpc).every(k=>['jsonrpc','id','method','params'].includes(k)))
      return [...safe,'-H','Content-Type: application/json','--data-raw',JSON.stringify(rpc),'--',origin.origin+'/rpc']
  }
  throw Error('Use a viewer discovery document or a read-only JSON-RPC method')
}

if (process.argv[1] && pathToFileURL(process.argv[1]).href === import.meta.url) {
  try {
    const args = viewerRequest(process.argv.slice(2), process.env.OTEL_EVAL_CURL_ENDPOINT)
    const child = spawnSync('/usr/bin/curl', args, { stdio:'inherit', env:{ LANG:'C' } })
    process.exitCode = child.status ?? 1
  } catch (e) { console.error(e.message); process.exitCode = 2 }
}
