// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import ThemeToggle from './ThemeToggle.svelte'

// document.documentElement persists across tests in this file, so reset the
// attribute the component owns before/after each test.
beforeEach(() => {
  document.documentElement.removeAttribute('data-theme')
})

afterEach(() => {
  document.documentElement.removeAttribute('data-theme')
})

describe('ThemeToggle', () => {
  it('defaults to Day when nothing is saved and the OS prefers light', () => {
    render(ThemeToggle)
    expect(document.documentElement.getAttribute('data-theme')).toBe(
      'madoromi-day'
    )
    expect(
      screen.getByRole('button', { name: 'Day theme active' })
    ).toBeInTheDocument()
    expect(localStorage.getItem('theme')).toBe('madoromi-day')
  })

  it('restores a previously saved theme from localStorage', () => {
    localStorage.setItem('theme', 'madoromi-night')
    render(ThemeToggle)
    expect(document.documentElement.getAttribute('data-theme')).toBe(
      'madoromi-night'
    )
    expect(
      screen.getByRole('button', { name: 'Night theme active' })
    ).toBeInTheDocument()
  })

  it('cycles to the next theme and persists it when clicked', async () => {
    render(ThemeToggle)
    await userEvent.click(screen.getByRole('button'))
    expect(document.documentElement.getAttribute('data-theme')).toBe(
      'madoromi-night'
    )
    expect(localStorage.getItem('theme')).toBe('madoromi-night')
    expect(
      screen.getByRole('button', { name: 'Night theme active' })
    ).toBeInTheDocument()
  })

  it('wraps back to Day after cycling through both themes', async () => {
    render(ThemeToggle)
    const button = screen.getByRole('button')
    await userEvent.click(button)
    await userEvent.click(button)
    expect(document.documentElement.getAttribute('data-theme')).toBe(
      'madoromi-day'
    )
    expect(localStorage.getItem('theme')).toBe('madoromi-day')
  })

  it.each([
    ['rose-pine-dawn', 'madoromi-day'],
    ['rose-pine-moon', 'madoromi-night'],
    ['rose-pine', 'madoromi-night'],
  ])('preserves the light/dark preference when migrating %s', (old, next) => {
    localStorage.setItem('theme', old)
    render(ThemeToggle)
    expect(document.documentElement.getAttribute('data-theme')).toBe(next)
    expect(localStorage.getItem('theme')).toBe(next)
  })
})
