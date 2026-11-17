import { tick } from 'svelte'

export type DetailFieldTarget = {
  source: string
  key: string
  kind: 'field' | 'attribute'
}

/** Reveal exact field identities without changing filters or other disclosures. */
export function revealDetailField(
  table: HTMLElement,
  initial: DetailFieldTarget | undefined
) {
  let cancel = () => {}
  function update(target: DetailFieldTarget | undefined) {
    cancel()
    let cancelled = false
    let timer: ReturnType<typeof setTimeout> | undefined
    let matches: HTMLElement[] = []
    cancel = () => {
      cancelled = true
      clearTimeout(timer)
      matches.forEach(row => row.classList.remove('detail-property--revealed'))
    }
    if (!target) return
    void tick().then(() => {
      if (cancelled) return
      matches = [...table.querySelectorAll<HTMLElement>('[data-field]')].filter(
        row =>
          row.dataset.field === target.key &&
          (row.dataset.fieldSource ?? table.dataset.fieldSource) ===
            target.source &&
          row.dataset.fieldKind === target.kind
      )
      const first = matches[0]
      if (!first) return
      for (
        let parent = first.parentElement;
        parent;
        parent = parent.parentElement
      ) {
        if (parent instanceof HTMLDetailsElement) parent.open = true
      }
      matches.forEach(row => row.classList.add('detail-property--revealed'))
      first.scrollIntoView({
        block: 'nearest',
        inline: 'nearest',
        behavior: window.matchMedia('(prefers-reduced-motion: reduce)').matches
          ? 'instant'
          : 'smooth',
      })
      timer = setTimeout(
        () =>
          matches.forEach(row =>
            row.classList.remove('detail-property--revealed')
          ),
        1800
      )
    })
  }
  update(initial)
  return { update, destroy: () => cancel() }
}
