/** Move and activate a tab without adding inactive tabs to the Tab sequence. */
export function handleTablistKeydown(
  event: KeyboardEvent & { currentTarget: HTMLDivElement }
) {
  const tablist = event.currentTarget
  const eventTarget = event.target
  if (!(eventTarget instanceof Element)) return
  const target = eventTarget.closest<HTMLButtonElement>('[role="tab"]')
  if (!target || !tablist.contains(target)) return

  const tabs = Array.from(
    tablist.querySelectorAll<HTMLButtonElement>('[role="tab"]:not(:disabled)')
  )
  const currentIndex = tabs.indexOf(target)
  if (currentIndex < 0 || tabs.length === 0) return

  let nextIndex: number
  if (event.key === 'ArrowRight') {
    nextIndex = (currentIndex + 1) % tabs.length
  } else if (event.key === 'ArrowLeft') {
    nextIndex = (currentIndex - 1 + tabs.length) % tabs.length
  } else if (event.key === 'Home') {
    nextIndex = 0
  } else if (event.key === 'End') {
    nextIndex = tabs.length - 1
  } else {
    return
  }

  event.preventDefault()
  const next = tabs[nextIndex]
  next.focus()
  next.click()
}
