// @vitest-environment jsdom
import { afterEach, describe, expect, it } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/svelte'
import AppearancePicker from './AppearancePicker.svelte'

afterEach(() => {
  document.documentElement.removeAttribute('data-theme')
  document.documentElement.removeAttribute('data-palette')
})

describe('AppearancePicker', () => {
  it('offers the three original themes and persists each choice', async () => {
    render(AppearancePicker)
    const trigger = screen.getByRole('button', {
      name: 'Appearance: Dawn Light',
    })
    for (const [label, theme] of [
      ['Moon Dark', 'rose-pine-moon'],
      ['Pine Dark', 'rose-pine'],
      ['Dawn Light', 'rose-pine-dawn'],
    ]) {
      await fireEvent.click(trigger)
      expect(screen.getAllByRole('radio')).toHaveLength(3)
      const choice = screen.getByRole('radio', { name: label })
      await fireEvent.click(choice)
      expect(choice).toBeChecked()
      expect(trigger).toHaveAccessibleName(`Appearance: ${label}`)
      expect(trigger).toHaveAttribute('aria-expanded', 'false')
      expect(trigger).toHaveFocus()
      expect(document.documentElement.dataset.theme).toBe(theme)
      expect(localStorage.getItem('theme')).toBe(theme)
    }
  })

  it.each([
    ['rose-pine-dawn', 'Dawn Light'],
    ['rose-pine-moon', 'Moon Dark'],
    ['rose-pine', 'Pine Dark'],
  ])('restores %s', (saved, label) => {
    localStorage.setItem('theme', saved)
    render(AppearancePicker)
    expect(
      screen.getByRole('button', { name: `Appearance: ${label}` })
    ).toBeInTheDocument()
    expect(document.documentElement.dataset.theme).toBe(saved)
  })

  it.each([
    ['madoromi-day', 'rose-pine-dawn'],
    ['madoromi-night', 'rose-pine-moon'],
  ])('keeps the light/dark preference when leaving %s', (saved, next) => {
    localStorage.setItem('theme', saved)
    localStorage.setItem('theme-palette', 'spring')
    document.documentElement.dataset.palette = 'spring'
    render(AppearancePicker)
    expect(document.documentElement.dataset.theme).toBe(next)
    expect(document.documentElement.dataset.palette).toBeUndefined()
    expect(localStorage.getItem('theme-palette')).toBeNull()
  })
})
