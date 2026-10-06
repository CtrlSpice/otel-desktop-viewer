import '@/fonts.css'
import '@/app.css'
import { mount } from 'svelte'
import App from '@/App.svelte'
import { initTooltipWarmth } from '@/utils/tooltip-warmth'
import { repairEmptyPersistedVisibleKeys } from '@/components/metrics/utils/metric-timeseries-visible'

// Run the versioned preference repair before reading metric view state.
repairEmptyPersistedVisibleKeys()

const target = document.getElementById('app')!
if (target) {
  mount(App, { target })
}

initTooltipWarmth()
