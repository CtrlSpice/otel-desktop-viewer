// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import ThemeToggle from './ThemeToggle.svelte'

// document.documentElement persists across tests in this file, so reset the
// attribute the component owns before/after each test.
beforeEach(() => {
  document.documentElement.removeAttribute('data-theme')
  document.documentElement.removeAttribute('data-color-scheme')
  localStorage.clear()
})

afterEach(() => {
  document.documentElement.removeAttribute('data-theme')
  document.documentElement.removeAttribute('data-color-scheme')
})

describe('ThemeToggle', () => {
  it('defaults to Winter Day when nothing is saved and the OS prefers light', () => {
    render(ThemeToggle)
    expect(document.documentElement.getAttribute('data-theme')).toBe(
      'madoromi-fuyu-hiru'
    )
    expect(
      screen.getByRole('button', { name: 'Winter Day theme active' })
    ).toBeInTheDocument()
    expect(localStorage.getItem('theme')).toBe('madoromi-fuyu-hiru')
    expect(
      screen.getByRole('button', { name: 'Season: Winter' })
    ).toBeInTheDocument()
  })

  it('restores a saved Winter Night theme', () => {
    localStorage.setItem('theme', 'madoromi-fuyu-yoru')
    render(ThemeToggle)
    expect(document.documentElement.getAttribute('data-theme')).toBe(
      'madoromi-fuyu-yoru'
    )
    expect(
      screen.getByRole('button', { name: 'Winter Night theme active' })
    ).toBeInTheDocument()
  })

  it('toggles day and night within the saved season', async () => {
    localStorage.setItem('theme', 'madoromi-aki-hiru')
    render(ThemeToggle)
    await userEvent.click(
      screen.getByRole('button', { name: 'Autumn Day theme active' })
    )
    expect(document.documentElement.getAttribute('data-theme')).toBe(
      'madoromi-aki-yoru'
    )
    expect(localStorage.getItem('theme')).toBe('madoromi-aki-yoru')
    expect(document.documentElement.getAttribute('data-color-scheme')).toBe(
      'dark'
    )
    expect(
      screen.getByRole('button', { name: 'Autumn Night theme active' })
    ).toBeInTheDocument()
  })

  it('changes season while retaining the mode, then restores both', async () => {
    localStorage.setItem('theme', 'madoromi-aki-yoru')
    const view = render(ThemeToggle)
    await userEvent.click(
      screen.getByRole('button', { name: 'Season: Autumn' })
    )
    await userEvent.click(screen.getByRole('button', { name: 'Summer' }))
    expect(document.documentElement.getAttribute('data-theme')).toBe(
      'madoromi-natsu-yoru'
    )
    expect(localStorage.getItem('theme')).toBe('madoromi-natsu-yoru')
    view.unmount()
    render(ThemeToggle)
    expect(
      screen.getByRole('button', { name: 'Season: Summer' })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Summer Night theme active' })
    ).toHaveAttribute('aria-pressed', 'true')
  })
})
