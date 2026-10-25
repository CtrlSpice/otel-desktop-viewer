// @vitest-environment jsdom
import { afterEach, expect, it, vi } from 'vitest'
import { graphColorOverrides } from './graph-color-overrides.svelte'
import {
  GRAPH_CURVE_STEPS,
  GRAPH_PALETTE_VERSION,
  GRAPH_SWATCHES,
} from '@/themes/graph-inks'

afterEach(() => vi.restoreAllMocks())

it('salvages typed override tuples while preserving exact identities and existing collisions', () => {
  const scope = 'saved-boundary'
  localStorage.setItem(
    `graph-colour-overrides:${GRAPH_PALETTE_VERSION}:${scope}`,
    JSON.stringify([
      ['', 'to-ro:1'],
      [' owner ', 'to-ro:1'],
      ['Owner', `to-ro:curve:${GRAPH_CURVE_STEPS}`],
      ['owner', 'to-ro:curve:0'],
      ['custom', '#Ab12EF'],
      ['short-hex', '#abc'],
      ['invalid-hex', '#zzzzzz'],
      ['unknown-ink', 'missing:1'],
      ['unknown-shade', 'to-ro:8'],
      ['outside', `to-ro:curve:${GRAPH_CURVE_STEPS + 1}`],
      ['wrong-type', 1],
      [42, 'to-ro:1'],
      ['extra', 'to-ro:1', 'ignored'],
      null,
    ])
  )
  expect(graphColorOverrides.entries(scope)).toEqual([
    ['', 'to-ro:1'],
    [' owner ', 'to-ro:1'],
    ['Owner', `to-ro:curve:${GRAPH_CURVE_STEPS}`],
    ['owner', 'to-ro:curve:0'],
    ['custom', '#ab12ef'],
  ])
  expect(graphColorOverrides.get(scope, '')?.color).toBe(
    graphColorOverrides.get(scope, ' owner ')?.color
  )
})

it('persists native colours, reserves matching presets and resets only the selected owner', () => {
  const scope = 'native-picker'
  const preset = GRAPH_SWATCHES[0]!
  graphColorOverrides.set(scope, 'first', preset.color)
  graphColorOverrides.set(scope, 'second', '#123456')
  expect(graphColorOverrides.reservedSlots(scope).has(0)).toBe(true)
  expect(graphColorOverrides.get(scope, 'first')?.color).toBe(
    preset.color.toLowerCase()
  )
  expect(
    JSON.parse(
      localStorage.getItem(
        `graph-colour-overrides:${GRAPH_PALETTE_VERSION}:${scope}`
      )!
    )
  ).toContainEqual(['second', '#123456'])
  graphColorOverrides.set(scope, 'first', 'not a colour')
  expect(graphColorOverrides.get(scope, 'first')?.color).toBe(
    preset.color.toLowerCase()
  )
  graphColorOverrides.set(scope, 'first', null)
  expect(graphColorOverrides.get(scope, 'first')).toBeUndefined()
  expect(graphColorOverrides.get(scope, 'second')?.color).toBe('#123456')
  expect(graphColorOverrides.reservedSlots(scope).has(0)).toBe(false)
})

it('merges hidden owners from storage and accepts cross-tab updates without normalizing identities', () => {
  const scope = 'cross-tab-boundary'
  const key = `graph-colour-overrides:${GRAPH_PALETTE_VERSION}:${scope}`
  graphColorOverrides.set(scope, 'visible', 'to-ro:1')
  localStorage.setItem(
    key,
    JSON.stringify([
      ['visible', 'to-ro:1'],
      ['hidden', 'to-ro:2'],
    ])
  )
  graphColorOverrides.set(scope, 'new', 'to-ro:3')
  expect(graphColorOverrides.entries(scope)).toContainEqual([
    'hidden',
    'to-ro:2',
  ])
  localStorage.setItem(
    key,
    JSON.stringify([
      [' Other ', 'to-ro:4'],
      ['invalid', false],
    ])
  )
  window.dispatchEvent(new StorageEvent('storage', { key }))
  expect(graphColorOverrides.entries(scope)).toEqual([[' Other ', 'to-ro:4']])
})

it('retains session overrides when storage writes are blocked', () => {
  vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
    throw new Error('blocked')
  })
  graphColorOverrides.set('blocked-boundary', 'owner', 'to-ro:1')
  expect(graphColorOverrides.get('blocked-boundary', 'owner')?.id).toBe(
    'to-ro:1'
  )
})
