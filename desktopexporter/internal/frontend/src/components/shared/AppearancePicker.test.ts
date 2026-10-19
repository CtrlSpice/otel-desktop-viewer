// @vitest-environment jsdom
import { afterEach, describe, expect, it } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/svelte'
import AppearancePicker from './AppearancePicker.svelte'

afterEach(() => {
  document.documentElement.removeAttribute('data-theme')
  document.documentElement.removeAttribute('data-palette')
})

describe('AppearancePicker', () => {
  it('selects each season/mode pair together and returns focus to the trigger', async () => {
    render(AppearancePicker)
    const trigger = screen.getByRole('button', {
      name: 'Appearance: Winter Light',
    })
    for (const season of ['Spring', 'Summer', 'Autumn', 'Winter']) {
      for (const [mode, theme] of [
        ['Light', 'madoromi-day'],
        ['Dark', 'madoromi-night'],
      ]) {
        await fireEvent.click(trigger)
        expect(screen.getAllByRole('radio')).toHaveLength(11)
        const choice = screen.getByRole('radio', { name: `${season} ${mode}` })
        await fireEvent.click(choice)
        expect(choice).toBeChecked()
        expect(trigger).toHaveAccessibleName(`Appearance: ${season} ${mode}`)
        expect(trigger).toHaveAttribute('aria-expanded', 'false')
        expect(trigger).toHaveFocus()
        expect(document.documentElement.dataset.theme).toBe(theme)
        expect(document.documentElement.dataset.palette).toBe(
          season.toLowerCase()
        )
        expect(localStorage.getItem('theme')).toBe(theme)
        expect(localStorage.getItem('theme-palette')).toBe(season.toLowerCase())
      }
    }
  })

  it('restores the saved pair', () => {
    localStorage.setItem('theme', 'madoromi-night')
    localStorage.setItem('theme-palette', 'summer')
    render(AppearancePicker)
    expect(
      screen.getByRole('button', { name: 'Appearance: Summer Dark' })
    ).toBeInTheDocument()
    expect(document.documentElement.dataset.palette).toBe('summer')
    expect(document.documentElement.dataset.theme).toBe('madoromi-night')
  })

  it.each([
    ['rose-pine-dawn', 'Dawn Light'],
    ['rose-pine-moon', 'Moon Dark'],
    ['rose-pine', 'Pine Dark'],
  ])('restores the original %s theme', (saved, label) => {
    localStorage.setItem('theme', saved)
    localStorage.setItem('theme-palette', 'spring')
    render(AppearancePicker)
    expect(document.documentElement.dataset.theme).toBe(saved)
    expect(document.documentElement.dataset.palette).toBeUndefined()
    expect(
      screen.getByRole('button', { name: `Appearance: ${label}` })
    ).toBeInTheDocument()
  })

  it('can switch between original themes and seasonal pairs', async () => {
    render(AppearancePicker)
    const trigger = screen.getByRole('button')
    for (const [label, theme] of [
      ['Dawn Light', 'rose-pine-dawn'],
      ['Moon Dark', 'rose-pine-moon'],
      ['Pine Dark', 'rose-pine'],
    ]) {
      await fireEvent.click(trigger)
      await fireEvent.click(screen.getByRole('radio', { name: label }))
      expect(localStorage.getItem('theme')).toBe(theme)
      expect(document.documentElement.dataset.palette).toBeUndefined()
    }
    await fireEvent.click(trigger)
    await fireEvent.click(screen.getByRole('radio', { name: 'Spring Light' }))
    expect(document.documentElement.dataset.theme).toBe('madoromi-day')
    expect(document.documentElement.dataset.palette).toBe('spring')
  })
})
