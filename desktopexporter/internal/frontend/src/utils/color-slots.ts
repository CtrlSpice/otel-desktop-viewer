import { SERIES_COLOR_COUNT, SERIES_PALETTE_VERSION } from '@/themes'
import { graphColorOverrides } from '@/state/graph-color-overrides.svelte'
import { colorSlotChanges } from '@/state/color-slot-changes.svelte'
import { GRAPH_INKS } from '@/themes/graph-inks'
import {
  AUTOMATIC_COLOR_VERSION,
  AUTOMATIC_COLOR_SLOTS,
} from '@/themes/service-ink-slots'

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
    if (preferred?.length === 0)
      throw new RangeError('The preferred colour pool must not be empty')
    const count = preferred?.length ?? this.size
    const start = colorIdentityHash(identity) % count
    let slot = preferred ? preferred[start]! : start
    for (let offset = 0; offset < count; offset++) {
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
const preparedScopes = new Set<string>()

function usesBrightRanking(scope: string, size: number): boolean {
  return (
    size === SERIES_COLOR_COUNT &&
    (scope === 'services' ||
      scope === 'span-names' ||
      scope === 'metric-series' ||
      scope.startsWith('metric:'))
  )
}

function allocationScope(scope: string, size: number): string {
  return usesBrightRanking(scope, size)
    ? `${scope}:${AUTOMATIC_COLOR_VERSION}`
    : scope
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
  return usesBrightRanking(scope, size)
    ? brightAllocator(scope)
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

function brightAllocator(scope: string): ColorSlotAllocator {
  const activeScope = allocationScope(scope, SERIES_COLOR_COUNT)
  const active = storedAllocator(activeScope, SERIES_COLOR_COUNT)
  if (preparedScopes.has(scope)) return active
  active.restore(
    loadSlots(storageKey(activeScope, SERIES_COLOR_COUNT), SERIES_COLOR_COUNT)
  )
  const legacy = loadSlots(
    storageKey(scope, SERIES_COLOR_COUNT),
    SERIES_COLOR_COUNT
  )
  // Carry owner order and hidden identities into the single palette once.
  // Prefer the previously active ink; leave old storage and manual choices intact.
  let previousInk: string | null = null
  try {
    previousInk = localStorage.getItem('theme-accent-ink')
  } catch {
    /* Storage can be blocked. */
  }
  const inks = GRAPH_INKS.map(ink => ink.id).sort(
    (a, b) => Number(b === previousInk) - Number(a === previousInk)
  )
  const previous = inks.flatMap(id =>
    [
      ':varied-3-2-v1',
      ':neighbours-3-2-v1',
      ':shades-3-1-v1',
      ':bright-3-2-v1',
      '',
    ].flatMap(version =>
      loadSlots(
        storageKey(`${scope}:ink:${id}${version}`, SERIES_COLOR_COUNT),
        SERIES_COLOR_COUNT
      )
    )
  )
  const before = active.snapshot().length
  const reserved = graphColorOverrides.reservedSlots(scope)
  for (const [identity] of [...previous, ...legacy]) {
    active.assign(identity, reserved, AUTOMATIC_COLOR_SLOTS)
  }
  preparedScopes.add(scope)
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
  const allocator = allocatorFor(scope, size)
  if (allocator.has(identity)) return allocator.assign(identity)
  const key = storageKey(allocationScope(scope, size), size)
  // Another tab may have assigned new identities since this registry was read.
  allocator.restore(loadSlots(key, size))
  if (allocator.has(identity)) return allocator.assign(identity)
  const slot = allocator.assign(
    identity,
    size === SERIES_COLOR_COUNT
      ? graphColorOverrides.reservedSlots(scope)
      : undefined,
    usesBrightRanking(scope, size) ? AUTOMATIC_COLOR_SLOTS : undefined
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
export function savedColorSlots(scope: string): [string, number][] {
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
  return allocator.snapshot()
}

if (typeof window !== 'undefined') {
  window.addEventListener('storage', event => {
    if (
      event.key === null ||
      event.key.startsWith(`graph-colour-slots:${SERIES_PALETTE_VERSION}:`)
    ) {
      preparedScopes.clear()
      colorSlotChanges.notify()
    }
  })
}
