// @vitest-environment jsdom
import { cleanup, fireEvent, screen } from '@testing-library/svelte'
import { afterEach, expect, it, vi } from 'vitest'
import { renderWithContexts } from '@/test/render-helpers'
import ThemePicker from './ThemePicker.svelte'
import { THEMES } from '@/themes'
import { GRAPH_SWATCHES } from '@/themes/graph-inks'
import { serviceInkSlots } from '@/themes/service-ink-slots'
import { accentInk } from '@/state/accent-ink.svelte'
import { graphColorOverrides } from '@/state/graph-color-overrides.svelte'
import { identityColorSlot, savedColorSlots } from '@/utils/color-slots'

afterEach(cleanup)

it('repaints sample services from the real ink ranking without allocating or reserving dummy identities', async () => {
  accentInk.set('to-ro')
  identityColorSlot('real service', 'services')
  identityColorSlot('real series', 'metric:preview-test')
  graphColorOverrides.set('services', 'manual service', 'rikka:4')
  const assignments = savedColorSlots('services')
  const graphStorage = () =>
    Object.fromEntries(
      Array.from({ length: localStorage.length }, (_, index) =>
        localStorage.key(index)!
      )
        .filter(key => key.startsWith('graph-colour-'))
        .map(key => [key, localStorage.getItem(key)])
    )
  const saved = graphStorage()
  renderWithContexts(ThemePicker, {
    theme: THEMES[0]!,
    onSeasonChange: vi.fn(),
  })
  await fireEvent.click(screen.getByRole('button', { name: 'Theme: To-ro' }))
  const checkout = screen.getByRole('img', { name: 'checkout-api: 6 spans' })
  const postgres = screen.getByRole('img', { name: 'postgres: 3 spans' })
  const payments = screen.getByRole('img', { name: 'payments: 1 span' })
  const queue = screen.getByRole('img', { name: 'queue: 1 span' })
  const color = (slot: number) => GRAPH_SWATCHES[slot]!.color
  expect(checkout.style.getPropertyValue('--part-color')).toBe(
    color(serviceInkSlots('to-ro')[0]!)
  )
  await fireEvent.click(screen.getByRole('radio', { name: 'Ajisai' }))
  expect(accentInk.value).toBe('ajisai')
  expect(checkout.style.getPropertyValue('--part-color')).toBe(
    color(serviceInkSlots('ajisai')[0]!)
  )
  expect(postgres.style.getPropertyValue('--part-color')).toBe(
    color(serviceInkSlots('ajisai')[1]!)
  )
  expect(payments.style.getPropertyValue('--part-color')).toBe(
    color(serviceInkSlots('ajisai')[3]!)
  )
  expect(queue.style.getPropertyValue('--part-color')).toBe(
    color(serviceInkSlots('ajisai')[4]!)
  )
  expect(graphStorage()).toEqual(saved)
  await fireEvent.click(screen.getByRole('radio', { name: 'To-ro' }))
  expect(checkout.style.getPropertyValue('--part-color')).toBe(
    color(serviceInkSlots('to-ro')[0]!)
  )
  expect(savedColorSlots('services')).toEqual(assignments)
  expect(graphStorage()).toEqual(saved)
})
