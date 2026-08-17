import '@/fonts.css'
import '@/app.css'
import { mount } from 'svelte'
import App from '@/App.svelte'
import { initTooltipWarmth } from '@/utils/tooltip-warmth'
import { repairEmptyPersistedVisibleKeys } from '@/components/metrics/utils/metric-timeseries-visible'

// Before anything reads stored view state: clears empty visible-key lists a
// previous build wrote by accident. Runs once, then never again, so a user who
// unticks every series keeps that choice.
repairEmptyPersistedVisibleKeys()

const target = document.getElementById('app')!
if (target) {
  mount(App, { target })
}

initTooltipWarmth()
