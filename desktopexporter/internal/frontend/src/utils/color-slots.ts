import { SERIES_COLOR_COUNT, SERIES_PALETTE_VERSION } from '@/themes'

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

  assign(identity: string): number {
    const existing = this.assigned.get(identity)
    if (existing !== undefined) return existing
    const start = colorIdentityHash(identity) % this.size
    let slot = start
    for (let offset = 0; offset < this.size; offset++) {
      const candidate = (start + offset) % this.size
      if (!this.occupied.has(candidate)) {
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

function storageKey(scope: string, size: number): string {
  return `graph-colour-slots:${SERIES_PALETTE_VERSION}:${size}:${scope}`
}

function loadSlots(key: string, size: number): [string, number][] {
  try {
    const raw = localStorage.getItem(key)
    if (!raw) return []
    const saved: unknown = JSON.parse(raw)
    if (!Array.isArray(saved)) return []
    const slots: [string, number][] = []
    for (const entry of saved) {
      // localStorage is an external boundary; reject malformed keys and slots.
      if (
        Array.isArray(entry) &&
        entry.length === 2 &&
        typeof entry[0] === 'string' &&
        typeof entry[1] === 'number' &&
        Number.isInteger(entry[1]) &&
        entry[1] >= 0 &&
        entry[1] < size
      ) {
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
  const key = storageKey(scope, size)
  let allocator = allocators.get(key)
  if (!allocator) {
    allocator = new ColorSlotAllocator(size, loadSlots(key, size))
    allocators.set(key, allocator)
  }
  return allocator
}

/** Saved per-origin assignments: services share a scope; series use their metric. */
export function identityColorSlot(
  identity: string,
  scope: string,
  size: number = SERIES_COLOR_COUNT
): number {
  const allocator = allocatorFor(scope, size)
  if (allocator.has(identity)) return allocator.assign(identity)
  const key = storageKey(scope, size)
  // Another tab may have assigned new identities since this registry was read.
  allocator.restore(loadSlots(key, size))
  if (allocator.has(identity)) return allocator.assign(identity)
  const slot = allocator.assign(identity)
  try {
    localStorage.setItem(key, JSON.stringify(allocator.snapshot()))
  } catch {
    // Retain the in-memory assignment when browser storage is unavailable/full.
  }
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
  return allocatorFor(scope, size).reuses()
}
