import '../src/fonts.css'
import '../src/app.css'
import { createArmABenchmarkAPI } from './arm-a'

const benchmarkSentinel = '__WATERFALL_BENCHMARK__'
const target = document.querySelector<HTMLDivElement>('#app')

if (!target) {
  throw new Error('benchmark mount target is missing')
}

target.dataset.benchmarkSentinel = benchmarkSentinel
target.textContent = 'Trace waterfall benchmark Arm A ready'
window.__TRACE_WATERFALL_BENCHMARK__ = createArmABenchmarkAPI(target)
