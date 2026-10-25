// @vitest-environment jsdom
import { fireEvent, screen } from '@testing-library/svelte'
import { expect, it, vi } from 'vitest'
import { renderWithContexts } from '@/test/render-helpers'
import ThemePicker from './ThemePicker.svelte'
import { THEMES } from '@/themes'
import { identityColorSlot, savedColorSlots } from '@/utils/color-slots'

it('offers four seasons without changing graph assignments', async () => {
  identityColorSlot('real service', 'services')
  const assignments = savedColorSlots('services')
  const onSeasonChange = vi.fn()
  renderWithContexts(ThemePicker, { theme: THEMES[0]!, onSeasonChange })
  await fireEvent.click(screen.getByRole('button', { name: 'Season: Haru' }))
  expect(screen.getAllByRole('radio')).toHaveLength(4)
  await fireEvent.click(screen.getByRole('radio', { name: 'Fuyu' }))
  expect(onSeasonChange).toHaveBeenCalledWith('fuyu')
  expect(savedColorSlots('services')).toEqual(assignments)
  expect(screen.getByRole('link', { name: 'Madoromi' })).toHaveAttribute(
    'href',
    'https://madoromi.moonwit.ch/'
  )
})
