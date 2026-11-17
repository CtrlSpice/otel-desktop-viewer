let version = $state(0)
let scheduled = false

export const colorSlotChanges = {
  get version() {
    return version
  },
  notify() {
    if (scheduled) return
    scheduled = true
    // Allocation can happen inside a chart's derived value. Notify picker
    // readers after that computation, rather than mutating reactive state in it.
    queueMicrotask(() => {
      scheduled = false
      version++
    })
  },
}
