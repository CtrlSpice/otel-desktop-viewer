import '@/fonts.css'
import '@/app.css'
import { mount } from 'svelte'
import App from '@/App.svelte'
import { initTooltipWarmth } from '@/utils/tooltip-warmth'

const target = document.getElementById('app')!
if (target) {
  mount(App, { target })
}

initTooltipWarmth()
