const views = ['result', 'time', 'cost']

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
