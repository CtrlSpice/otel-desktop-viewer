import { startDrag, type DragHandle } from './drag'
import {
  remToPx,
  resolvePanelResize,
  PANEL_CLOSE_OVERSHOOT_REM,
} from '@/state/panel-width'

export const DETAIL_STACK = Symbol('detail-stack')

type Section = {
  id: string
  label: string
  layoutKey?: string
  element: HTMLElement
  available: () => boolean
  expanded: () => boolean
  setExpanded: (open: boolean) => void
}

function isStoredWeights(value: unknown): value is object {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
}

function isPositiveWeight(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value) && value > 0
}

function loadWeights(key: string): Record<string, number> {
  try {
    const value: unknown = JSON.parse(localStorage.getItem(key) ?? 'null')
    if (!isStoredWeights(value)) return {}
    return Object.fromEntries(
      Object.entries(value).filter(([, weight]) => isPositiveWeight(weight))
    )
  } catch {
    return {}
  }
}

/** Shared panel dragging with ordered content transfer:
 * fixed headers → nearest receiver → nearest open donor → resistance → next donor.
 * Open sections fill the stack; only the resize controller commits drag state.
 */
export function createDetailStackState(storageKey: string) {
  let sections = $state<Section[]>([])
  let weights = $state(loadWeights(storageKey))
  let dragging = $state<string | null>(null)
  let previewSizes = $state<Record<string, number> | null>(null)
  let drag: DragHandle | undefined

  function next(id: string) {
    return sections[sections.findIndex(section => section.id === id) + 1]
  }

  function sides(id: string) {
    const boundary = sections.findIndex(section => section.id === id)
    if (boundary < 0) return
    const above = sections
      .slice(0, boundary + 1)
      .filter(section => section.available())
      .reverse()
    const below = sections
      .slice(boundary + 1)
      .filter(section => section.available())
    return above.length && below.length ? { above, below } : undefined
  }

  function collapseReceiver(id: string) {
    const index = sections.findIndex(section => section.id === id)
    if (index < 0) return
    const below = sections.slice(index + 1)
    const above = sections.slice(0, index).reverse()
    const candidates =
      index <= (sections.length - 1) / 2
        ? [...below, ...above]
        : [...above, ...below]
    return candidates.find(section => section.available())
  }

  function canClose(id: string) {
    return Boolean(collapseReceiver(id))
  }

  function setOpen(id: string, open: boolean) {
    const section = sections.find(candidate => candidate.id === id)
    if (!section?.available()) return
    drag?.cancel()
    if (open) {
      section.setExpanded(true)
      return
    }
    const receiver = collapseReceiver(id)
    if (!receiver) {
      section.setExpanded(true)
      return
    }
    if (!section.expanded()) return
    // A heading collapse hands its entire allocation to one neighbour.
    // Open the receiver first so the stack never has an all-closed state.
    const received =
      (receiver.expanded() ? weight(receiver) : 0) + weight(section)
    weights = { ...weights, [receiver.layoutKey ?? receiver.label]: received }
    receiver.setExpanded(true)
    section.setExpanded(false)
    save()
  }

  function weight(section: Section) {
    return (
      weights[section.layoutKey ?? section.label] ??
      1 /
        Math.max(1, sections.filter(candidate => candidate.available()).length)
    )
  }

  function save() {
    try {
      localStorage.setItem(storageKey, JSON.stringify(weights))
    } catch {
      /* Keep session sizing when storage is unavailable. */
    }
  }

  function snapshot(id: string) {
    const groups = sides(id)
    if (!groups) return
    const { above, below } = groups
    // Headers never move out of order. Only the content space crosses a divider.
    const headers = new Map(
      sections.map(section => [
        section.id,
        section.element
          .querySelector('.detail-bubble__heading')
          ?.getBoundingClientRect().height ?? remToPx(2.5),
      ])
    )
    const initial = new Map(
      sections.map(section => {
        const height = section.expanded()
          ? Math.max(
              0,
              section.element.getBoundingClientRect().height -
                headers.get(section.id)!
            )
          : 0
        return [section.id, height] as const
      })
    )
    const total = [...initial.values()].reduce((sum, height) => sum + height, 0)
    if (total <= 0) return
    let lastDelta = 0
    const remembered = { ...weights }

    function commit(sizes: Map<string, number>, settle: boolean) {
      const updated = { ...remembered }
      for (const section of sections) {
        if (!section.available()) continue
        const height = sizes.get(section.id) ?? 0
        // Keep the last useful size for reopening with the heading.
        if (height > 0)
          updated[section.layoutKey ?? section.label] = height / total
        section.setExpanded(height > 0)
      }
      if (settle) {
        weights = updated
        previewSizes = null
      } else {
        // Match the page resizer's preview/commit split. Fixed pixel previews
        // keep unrelated sections still instead of making flex growers compete.
        previewSizes = Object.fromEntries(
          [...sizes].map(([sectionID, height]) => [
            sectionID,
            height + headers.get(sectionID)!,
          ])
        )
      }
    }

    function apply(delta: number, settle = false) {
      lastDelta = delta
      const sizes = new Map(initial)
      const receivers = delta >= 0 ? above : below
      const donors = delta >= 0 ? below : above
      const receiver = receivers[0]!
      let remaining = Math.abs(delta)
      for (const donor of donors) {
        const height = sizes.get(donor.id) ?? 0
        if (height <= 0 || remaining <= 0) continue
        const resized = resolvePanelResize(
          height - remaining,
          Math.min(remToPx(3.5), height),
          total,
          remToPx(PANEL_CLOSE_OVERSHOOT_REM)
        )
        const nextHeight = settle
          ? resized.collapsed
            ? 0
            : resized.size
          : resized.preview
        const transfer = height - nextHeight
        sizes.set(donor.id, nextHeight)
        sizes.set(receiver.id, (sizes.get(receiver.id) ?? 0) + transfer)
        remaining = resized.remainingCloseTravel
      }
      commit(sizes, settle)
    }

    return {
      total,
      apply,
      finish: () => apply(lastDelta, true),
    }
  }

  return {
    get dragging() {
      return dragging
    },
    register(section: Section) {
      sections = [...sections, section]
      return () => {
        drag?.cancel()
        sections = sections.filter(candidate => candidate.id !== section.id)
      }
    },
    next,
    canClose,
    setOpen,
    ensureOpen() {
      if (!sections.some(section => section.expanded())) {
        sections.find(section => section.available())?.setExpanded(true)
      }
    },
    enabled: (id: string) => Boolean(sides(id)),
    flex(id: string) {
      if (previewSizes?.[id] !== undefined) return `0 0 ${previewSizes[id]}px`
      const section = sections.find(candidate => candidate.id === id)
      if (!section?.expanded()) return '0 0 var(--pane-header-h)'
      const total = sections
        .filter(candidate => candidate.expanded())
        .reduce((sum, candidate) => sum + weight(candidate), 0)
      return `${(100 * weight(section)) / total} 1 var(--pane-header-h)`
    },
    percent(id: string) {
      const groups = sides(id)
      if (!groups) return 0
      const sum = (group: Section[]) =>
        group.reduce(
          (total, section) =>
            total + (section.expanded() ? weight(section) : 0),
          0
        )
      const above = sum(groups.above)
      return Math.round((above / (above + sum(groups.below))) * 100)
    },
    start(id: string, event: PointerEvent) {
      if (event.button !== 0) return
      drag?.cancel()
      const sizes = snapshot(id)
      if (!sizes) return
      dragging = id
      sizes.apply(0)
      drag = startDrag(event, {
        axis: 'y',
        onMove: delta => sizes.apply(delta),
        onEnd: () => {
          sizes.finish()
          dragging = null
          drag = undefined
          save()
        },
      })
    },
    keydown(id: string, event: KeyboardEvent) {
      if (!['ArrowUp', 'ArrowDown', 'Home', 'End'].includes(event.key)) return
      const sizes = snapshot(id)
      if (!sizes) return
      event.preventDefault()
      const step = sizes.total * (event.shiftKey ? 0.05 : 0.01)
      sizes.apply(
        event.key === 'Home'
          ? Number.NEGATIVE_INFINITY
          : event.key === 'End'
            ? Number.POSITIVE_INFINITY
            : event.key === 'ArrowUp'
              ? -step
              : step
      )
      sizes.finish()
      save()
    },
    reset() {
      drag?.cancel()
      weights = {}
      save()
    },
    destroy() {
      drag?.cancel()
    },
  }
}

export type DetailStackState = ReturnType<typeof createDetailStackState>
