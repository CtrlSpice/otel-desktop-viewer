import { SERIES_COLOR_COUNT, SERIES_PALETTE_VERSION } from '@/themes'
import { graphColorOverrides } from '@/state/graph-color-overrides.svelte'
import { colorSlotChanges } from '@/state/color-slot-changes.svelte'
import { accentInk } from '@/state/accent-ink.svelte'
import { ACCENT_INKS, type AccentInkId } from '@/themes/accent-inks'
import { serviceInkSlots } from '@/themes/service-ink-slots'

export type ColorSlotReuse = { slot: number; identities: string[] }

/** Exact identity, FNV-1a, then a 32-bit avalanche; no case/whitespace folding. */
export function colorIdentityHash(identity: string): number {
  let hash = 2166136261
  for (let i = 0; i < identity.length; i++) {
    hash = Math.imul(hash ^ identity.charCodeAt(i), 16777619)
  }
  hash ^= hash >>> 16
  hash = Math.imul(hash, 0x85ebca6b)
  hash ^= hash >>> 13
  hash = Math.imul(hash, 0xc2b2ae35)
  return (hash ^ (hash >>> 16)) >>> 0
}

/** Retains logical slots, not theme colours. Exhaustion never moves an owner. */
export class ColorSlotAllocator {
  private readonly assigned = new Map<string, number>()
  private readonly occupied = new Set<number>()

  constructor(
    readonly size: number,
    saved: Iterable<readonly [string, number]> = []
  ) {
    if (!Number.isSafeInteger(size) || size <= 0) {
      throw new RangeError('The colour-slot count must be a positive integer')
    }
    this.restore(saved)
  }

  /** Merge saved owners from another tab without moving an existing owner. */
  restore(saved: Iterable<readonly [string, number]>): void {
    for (const [identity, slot] of saved) {
      if (this.assigned.has(identity)) continue
      this.assigned.set(identity, slot)
      this.occupied.add(slot)
    }
  }

  has(identity: string): boolean {
    return this.assigned.has(identity)
  }

  assign(
    identity: string,
    reserved: ReadonlySet<number> = new Set(),
    preferred?: readonly number[]
  ): number {
    const existing = this.assigned.get(identity)
    if (existing !== undefined) return existing
    const start = colorIdentityHash(identity) % this.size
    let slot = start
    for (let offset = 0; offset < this.size; offset++) {
      const candidate = preferred
        ? preferred[offset]!
        : (start + offset) % this.size
      if (!this.occupied.has(candidate) && !reserved.has(candidate)) {
        slot = candidate
        break
      }
    }
    this.assigned.set(identity, slot)
    this.occupied.add(slot)
    return slot
  }

  snapshot(): [string, number][] {
    return [...this.assigned]
  }

  reuses(): ColorSlotReuse[] {
    const owners = new Map<number, string[]>()
    for (const [identity, slot] of this.assigned) {
      const names = owners.get(slot)
      if (names) names.push(identity)
      else owners.set(slot, [identity])
    }
    const reuses: ColorSlotReuse[] = []
    for (const [slot, identities] of owners) {
      if (identities.length > 1) reuses.push({ slot, identities })
    }
    return reuses
  }
}

const allocators = new Map<string, ColorSlotAllocator>()
const reportedReuse = new Set<string>()
const preparedInks = new Map<string, AccentInkId>()

function usesInkRanking(scope: string, size: number): boolean {
  return (
    size === SERIES_COLOR_COUNT &&
    (scope === 'services' ||
      scope === 'span-names' ||
      scope === 'metric-series' ||
      scope.startsWith('metric:'))
  )
}

function allocationScope(scope: string, size: number): string {
  return usesInkRanking(scope, size) ? `${scope}:ink:${accentInk.value}` : scope
}

function storageKey(scope: string, size: number): string {
  return `graph-colour-slots:${SERIES_PALETTE_VERSION}:${size}:${scope}`
}

function isSavedSlot(value: unknown): value is [string, number] {
  return (
    Array.isArray(value) &&
    value.length === 2 &&
    typeof value[0] === 'string' &&
    typeof value[1] === 'number'
  )
}

function loadSlots(key: string, size: number): [string, number][] {
  try {
    const raw = localStorage.getItem(key)
    if (!raw) return []
    const saved: unknown = JSON.parse(raw)
    if (!Array.isArray(saved)) return []
    const slots: [string, number][] = []
    for (const entry of saved.filter(isSavedSlot)) {
      // localStorage is an external boundary; reject malformed keys and slots.
      if (Number.isInteger(entry[1]) && entry[1] >= 0 && entry[1] < size) {
        slots.push([entry[0], entry[1]])
      }
    }
    return slots
  } catch {
    // A blocked or malformed store costs persistence, not the current view.
    return []
  }
}

function allocatorFor(scope: string, size: number): ColorSlotAllocator {
  return usesInkRanking(scope, size)
    ? inkAllocator(scope)
    : storedAllocator(scope, size)
}

function storedAllocator(scope: string, size: number): ColorSlotAllocator {
  const key = storageKey(scope, size)
  let allocator = allocators.get(key)
  if (!allocator) {
    allocator = new ColorSlotAllocator(size, loadSlots(key, size))
    allocators.set(key, allocator)
  }
  return allocator
}

function persistSlots(key: string, allocator: ColorSlotAllocator): void {
  colorSlotChanges.notify()
  try {
    localStorage.setItem(key, JSON.stringify(allocator.snapshot()))
  } catch {
    // Retain the in-memory assignment when browser storage is unavailable/full.
  }
}

function inkAllocator(scope: string): ColorSlotAllocator {
  const ink = accentInk.value
  const activeScope = allocationScope(scope, SERIES_COLOR_COUNT)
  const active = storedAllocator(activeScope, SERIES_COLOR_COUNT)
  if (preparedInks.get(scope) === ink) return active
  const saved = ACCENT_INKS.flatMap(candidate => {
    const candidateScope = `${scope}:ink:${candidate.id}`
    const allocator = storedAllocator(candidateScope, SERIES_COLOR_COUNT)
    allocator.restore(
      loadSlots(
        storageKey(candidateScope, SERIES_COLOR_COUNT),
        SERIES_COLOR_COUNT
      )
    )
    return allocator.snapshot()
  })
  const legacy = loadSlots(
    storageKey(scope, SERIES_COLOR_COUNT),
    SERIES_COLOR_COUNT
  )
  const before = active.snapshot().length
  // Adopt existing assignments for the initially selected ink, keeping the old
  // storage intact. Other inks replay all known owners, including hidden ones.
  if (saved.length === 0) active.restore(legacy)
  const reserved = graphColorOverrides.reservedSlots(scope)
  const preferred = serviceInkSlots(ink)
  for (const [identity] of [...saved, ...legacy]) {
    active.assign(identity, reserved, preferred)
  }
  preparedInks.set(scope, ink)
  if (active.snapshot().length !== before)
    persistSlots(storageKey(activeScope, SERIES_COLOR_COUNT), active)
  return active
}

/** Saved per-origin assignments: services share a scope; series use their metric. */
export function identityColorSlot(
  identity: string,
  scope: string,
  size: number = SERIES_COLOR_COUNT
): number {
  let allocator = allocatorFor(scope, size)
  if (allocator.has(identity)) return allocator.assign(identity)
  if (usesInkRanking(scope, size)) {
    preparedInks.delete(scope)
    allocator = inkAllocator(scope)
  }
  const key = storageKey(allocationScope(scope, size), size)
  // Another tab may have assigned new identities since this registry was read.
  allocator.restore(loadSlots(key, size))
  if (allocator.has(identity)) return allocator.assign(identity)
  const slot = allocator.assign(
    identity,
    size === SERIES_COLOR_COUNT
      ? graphColorOverrides.reservedSlots(scope)
      : undefined,
    usesInkRanking(scope, size) ? serviceInkSlots(accentInk.value) : undefined
  )
  persistSlots(key, allocator)
  if (!reportedReuse.has(key) && allocator.reuses().length > 0) {
    reportedReuse.add(key)
    console.warn(
      `Graph colour slots exhausted in ${scope}; colours are being reused. Existing assignments are unchanged.`
    )
  }
  return slot
}

/** Diagnostics only; reading reuse does not allocate or recolour an identity. */
export function colorSlotReuses(
  scope: string,
  size: number = SERIES_COLOR_COUNT
): ColorSlotReuse[] {
  return storedAllocator(allocationScope(scope, size), size).reuses()
}

/** All saved owners, including hidden services/series and their reset slots. */
export function savedColorSlots(
  scope: string,
  includeInactiveInks = false
): [string, number][] {
  void colorSlotChanges.version
  const allocator = allocatorFor(scope, SERIES_COLOR_COUNT)
  allocator.restore(
    loadSlots(
      storageKey(
        allocationScope(scope, SERIES_COLOR_COUNT),
        SERIES_COLOR_COUNT
      ),
      SERIES_COLOR_COUNT
    )
  )
  if (usesInkRanking(scope, SERIES_COLOR_COUNT) && includeInactiveInks) {
    return [
      ...allocator.snapshot(),
      ...ACCENT_INKS.filter(ink => ink.id !== accentInk.value).flatMap(ink =>
        storedAllocator(`${scope}:ink:${ink.id}`, SERIES_COLOR_COUNT).snapshot()
      ),
    ]
  }
  return allocator.snapshot()
}

if (typeof window !== 'undefined') {
  window.addEventListener('storage', event => {
    if (
      event.key === null ||
      event.key.startsWith(`graph-colour-slots:${SERIES_PALETTE_VERSION}:`)
    ) {
      preparedInks.clear()
      colorSlotChanges.notify()
    }
  })
}
