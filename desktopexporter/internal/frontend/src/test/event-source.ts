import { tick } from 'svelte'

export class TestEventSource extends EventTarget {
  static connections = new Set<TestEventSource>()
  onerror: (() => void) | null = null
  constructor(readonly url: string) {
    super()
    TestEventSource.connections.add(this)
  }
  close() {
    TestEventSource.connections.delete(this)
  }
}

export async function notifyDataChanged() {
  for (const source of TestEventSource.connections)
    source.dispatchEvent(new Event('change'))
  await tick()
  await tick()
}
