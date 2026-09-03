import '../src/fonts.css'
import '../src/app.css'
import { createTraceWaterfallBenchmarkAPI } from './benchmark-api'

const benchmarkSentinel = '__WATERFALL_BENCHMARK__'
const target = document.querySelector<HTMLDivElement>('#app')

if (!target) {
  throw new Error('benchmark mount target is missing')
}

target.dataset.benchmarkSentinel = benchmarkSentinel
target.textContent = 'Trace waterfall benchmark Arms A and C ready'
window.__TRACE_WATERFALL_BENCHMARK__ = createTraceWaterfallBenchmarkAPI(target)
