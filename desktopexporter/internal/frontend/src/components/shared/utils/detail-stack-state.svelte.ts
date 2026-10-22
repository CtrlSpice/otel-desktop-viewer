import { startDrag, type DragHandle } from './drag'
import { tick } from 'svelte'
import {
  remToPx,
  resolvePanelResize,
  PANEL_CLOSE_OVERSHOOT_REM,
} from '@/state/panel-width'

export const DETAIL_STACK = Symbol('detail-stack')
export const DETAIL_SECTION = Symbol('detail-section')

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

/** One layout owner: the primary section stays open; remaining sections group
 * as open, closed available, then inactive.
 * Keys retain mounted content. Only open neighbours exchange space during drag;
 * disclosure/group changes commit on release. */
export function createDetailStackState(storageKey: string) {
  let sections = $state<Section[]>([])
  let weights = $state(loadWeights(storageKey))
  let dragging = $state<string | null>(null)
  let previewSizes = $state<Record<string, number> | null>(null)
  let drag: DragHandle | undefined
  let order: string[] = []

  const opened = () =>
    sections.filter(section => section.available() && section.expanded())
  const primary = () =>
    sections.find(section => section.id === order[0]) ?? sections[0]
  const group = (section?: Section) =>
    !section ? 0 : !section.available() ? 2 : section.expanded() ? 0 : 1
  function preserveFocus() {
    const focused =
      document.activeElement instanceof HTMLElement &&
      sections.some(section => section.element.contains(document.activeElement))
        ? document.activeElement
        : undefined
    void tick().then(() => {
      if (focused?.isConnected && document.activeElement === document.body)
        focused.focus({ preventScroll: true })
    })
  }

  function previous(id: string) {
    const index = sections.findIndex(section => section.id === id)
    return sections
      .slice(0, index)
      .findLast(section => section.available() && section.expanded())
  }

  function sides(id: string) {
    const open = opened()
    const boundary = open.findIndex(section => section.id === id)
    if (boundary < 0) return
    const above = open[boundary]!
    const below = open[boundary + 1]
    return below ? { above, below } : undefined
  }

  function canClose(id: string) {
    return id !== primary()?.id && Boolean(previous(id))
  }

  function setOpen(id: string, open: boolean) {
    const section = sections.find(candidate => candidate.id === id)
    if (!section?.available()) return
    drag?.cancel()
    if (!open && !canClose(id)) {
      section.setExpanded(true)
      return
    }
    if (section.expanded() === open) {
      section.setExpanded(open)
      return
    }
    preserveFocus()
    if (open) {
      const current = opened()
      const total = current.reduce((sum, item) => sum + weight(item), 0)
      const contentHeight = current.reduce(
        (sum, item) =>
          sum +
          Math.max(
            0,
            item.element.getBoundingClientRect().height -
              (item.element
                .querySelector('.detail-bubble__heading')
                ?.getBoundingClientRect().height ?? 0)
          ),
        0
      )
      const minimum =
        contentHeight > 0
          ? Math.min(1 / (current.length + 1), remToPx(3.5) / contentHeight)
          : 0
      const remembered =
        weights[section.layoutKey ?? section.label] ?? 1 / (current.length + 1)
      const desired = Math.min(
        1 - minimum * current.length,
        remembered > 1 ? remembered / (total + remembered) : remembered
      )
      const updated = { ...weights }
      for (const item of current)
        updated[item.layoutKey ?? item.label] = weight(item) / total
      let remaining = desired
      const index = sections.indexOf(section)
      const donors = [
        ...current.filter(item => sections.indexOf(item) < index).reverse(),
        ...current.filter(item => sections.indexOf(item) > index),
      ]
      for (const donor of donors) {
        const key = donor.layoutKey ?? donor.label
        const transfer = Math.min(
          remaining,
          Math.max(0, updated[key] - minimum)
        )
        updated[key] -= transfer
        remaining -= transfer
      }
      updated[section.layoutKey ?? section.label] = current.length
        ? desired - remaining
        : 1
      weights = updated
      section.setExpanded(true)
      save()
      return
    }
    const receiver = previous(id)!
    const total = opened().reduce((sum, item) => sum + weight(item), 0)
    const updated = { ...weights }
    for (const item of opened())
      updated[item.layoutKey ?? item.label] = weight(item) / total
    updated[receiver.layoutKey ?? receiver.label] +=
      updated[section.layoutKey ?? section.label]
    weights = updated
    section.setExpanded(false)
    save()
  }

  function weight(section: Section) {
    return (
      weights[section.layoutKey ?? section.label] ??
      1 / Math.max(1, opened().length)
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
    // Group/order stays fixed until release. Only open content crosses the boundary.
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
        // Closed and inactive headers never contribute resize space.
        const height =
          section.available() && section.expanded()
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
    for (const section of opened())
      remembered[section.layoutKey ?? section.label] =
        initial.get(section.id)! / total
    const layoutKeys = new Map(
      sections.map(section => [section.id, section.layoutKey ?? section.label])
    )

    function preview(sizes: Map<string, number>) {
      // Fixed pixel previews keep unrelated sections still during resizing.
      previewSizes = Object.fromEntries(
        [...sizes].map(([sectionID, height]) => [
          sectionID,
          height + headers.get(sectionID)!,
        ])
      )
    }

    function commit(sizes: Map<string, number>, settle: boolean) {
      const updated = { ...remembered }
      for (const section of sections) {
        if (!section.available() || !section.expanded()) continue
        const height = sizes.get(section.id) ?? 0
        // Keep the last useful size for reopening with the heading.
        if (height > 0) updated[layoutKeys.get(section.id)!] = height / total
        if (settle) section.setExpanded(height > 0)
      }
      if (settle) {
        weights = updated
        previewSizes = null
      } else {
        preview(sizes)
      }
    }

    function apply(delta: number, settle = false) {
      lastDelta = delta
      const sizes = new Map(initial)
      const receiver = delta >= 0 ? above : below
      const donor = delta >= 0 ? below : above
      const height = initial.get(donor.id)!
      const resized = resolvePanelResize(
        height - Math.abs(delta),
        Math.min(remToPx(3.5), height),
        total,
        remToPx(PANEL_CLOSE_OVERSHOOT_REM)
      )
      const nextHeight =
        donor.id === primary()?.id
          ? resized.size
          : settle
            ? resized.collapsed
              ? 0
              : resized.size
            : resized.preview
      sizes.set(donor.id, nextHeight)
      sizes.set(receiver.id, initial.get(receiver.id)! + height - nextHeight)
      commit(sizes, settle)
    }

    return {
      total,
      apply,
      finish: () => apply(lastDelta, true),
    }
  }

  return {
    setOrder(keys: string[]) {
      order = keys
      sections = [...sections].sort(
        (a, b) => order.indexOf(a.id) - order.indexOf(b.id)
      )
    },
    ordered<T extends { id: string }>(slots: T[]) {
      return [...slots].sort(
        (a, b) =>
          group(sections.find(section => section.id === a.id)) -
          group(sections.find(section => section.id === b.id))
      )
    },
    setStorageKey(key: string) {
      if (key === storageKey) return
      // Finish any resize in its original layout before loading the next one.
      drag?.cancel()
      previewSizes = null
      storageKey = key
      weights = loadWeights(key)
    },
    get dragging() {
      return dragging
    },
    register(section: Section) {
      sections = [...sections, section].sort(
        (a, b) => order.indexOf(a.id) - order.indexOf(b.id)
      )
      return () => {
        drag?.cancel()
        sections = sections.filter(candidate => candidate.id !== section.id)
      }
    },
    previous,
    canClose,
    setOpen,
    ensureOpen() {
      const first = primary()
      if (first?.available() && !first.expanded()) first.setExpanded(true)
      if (!opened().length) {
        sections.find(section => section.available())?.setExpanded(true)
      }
    },
    enabled: (id: string) => Boolean(sides(id)),
    flex(id: string) {
      if (previewSizes?.[id] !== undefined) return `0 0 ${previewSizes[id]}px`
      const section = sections.find(candidate => candidate.id === id)
      if (!section?.expanded()) return '0 0 var(--pane-header-h)'
      const total = sections
        .filter(candidate => candidate.available() && candidate.expanded())
        .reduce((sum, candidate) => sum + weight(candidate), 0)
      return `${(100 * weight(section)) / total} 1 var(--pane-header-h)`
    },
    start(id: string, event: PointerEvent, onDrag: () => void) {
      if (event.button !== 0) return
      drag?.cancel()
      const sizes = snapshot(id)
      if (!sizes) return
      dragging = id
      let moved = false
      drag = startDrag(event, {
        axis: 'y',
        cursor: 'grabbing',
        onMove: delta => {
          if (!moved) {
            if (Math.abs(delta) < 4) return
            moved = true
            onDrag()
          }
          sizes.apply(delta)
        },
        onEnd: () => {
          preserveFocus()
          if (moved) {
            sizes.finish()
            save()
          } else previewSizes = null
          dragging = null
          drag = undefined
        },
      })
    },
    keydown(id: string, event: KeyboardEvent) {
      if (event.key === '0') {
        event.preventDefault()
        drag?.cancel()
        weights = {}
        save()
        return
      }
      if (!['ArrowUp', 'ArrowDown', 'Home', 'End'].includes(event.key)) return
      drag?.cancel()
      const sizes = snapshot(id)
      if (!sizes) return
      preserveFocus()
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
    destroy() {
      drag?.cancel()
    },
  }
}

export type DetailStackState = ReturnType<typeof createDetailStackState>
