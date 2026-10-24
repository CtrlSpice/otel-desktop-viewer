// @vitest-environment jsdom
import { afterEach, expect, it, vi } from 'vitest'
import { graphColorOverrides } from './graph-color-overrides.svelte'
import { GRAPH_CURVE_STEPS, GRAPH_PALETTE_VERSION } from '@/themes/graph-inks'

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
  ])
  expect(graphColorOverrides.get(scope, '')?.color).toBe(
    graphColorOverrides.get(scope, ' owner ')?.color
  )
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
