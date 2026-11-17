import type { Page } from '@playwright/test'

export async function mockDataChanges(page: Page) {
  await page.addInitScript(() => {
    class Changes extends EventTarget {
      onerror: (() => void) | null = null
      changed = () =>
        this.dispatchEvent(new MessageEvent('change', { data: '1' }))
      disconnected = () => this.onerror?.()
      constructor() {
        super()
        window.addEventListener('test-commit', this.changed)
        window.addEventListener('test-disconnect', this.disconnected)
      }
      close() {
        window.removeEventListener('test-commit', this.changed)
        window.removeEventListener('test-disconnect', this.disconnected)
      }
    }
    Object.defineProperty(window, 'EventSource', { value: Changes })
  })
}

export async function notifyDataChanged(page: Page) {
  await page.evaluate(() => window.dispatchEvent(new Event('test-commit')))
}
