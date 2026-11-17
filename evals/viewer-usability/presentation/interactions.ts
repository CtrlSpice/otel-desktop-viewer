const views = ['result', 'time', 'cost']

const localDateTime = new Intl.DateTimeFormat('en-CA', {
  year: 'numeric', month: '2-digit', day: '2-digit',
  hour: '2-digit', minute: '2-digit', second: '2-digit',
  timeZoneName: 'short',
  hourCycle: 'h23',
})

function localizeTimes() {
  for (const time of document.querySelectorAll<HTMLTimeElement>('time[data-local-time]')) {
    const parts = Object.fromEntries(localDateTime.formatToParts(new Date(time.dateTime))
      .map(part => [part.type, part.value]))
    time.textContent = `${parts.year}-${parts.month}-${parts.day} ${parts.hour}:${parts.minute}:${parts.second} ${parts.timeZoneName}`
  }
}
localizeTimes()
document.addEventListener('eval-results-updated', localizeTimes)

// Delegation also handles rows replaced by the saved-results live prototype.
document.addEventListener('click', event => {
  if (!(event.target instanceof Element)) return

  const tableButton =
    event.target.closest<HTMLButtonElement>('button[data-grading-table]') ??
    event.target
      .closest('.grading-model-header')
      ?.querySelector<HTMLButtonElement>('button[data-grading-table]')
  if (tableButton) {
    const body = document.getElementById(tableButton.dataset.gradingTable ?? '')
    if (body) {
      const expanded = tableButton.getAttribute('aria-expanded') !== 'true'
      tableButton.setAttribute('aria-expanded', String(expanded))
      body.hidden = !expanded
    }
    return
  }

  const button = event.target.closest<HTMLButtonElement>('button[data-view]')
  const view = button?.dataset.view
  if (!button || !view || !views.includes(view)) return
  document.body.dataset.metric = view
  for (const item of document.querySelectorAll('[data-view]')) {
    item.setAttribute('aria-pressed', String(item === button))
  }
})
