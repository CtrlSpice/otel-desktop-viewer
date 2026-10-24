import plugin from 'tailwindcss/plugin'
import { THEMES } from './index'
import { themeTokens, accentInkTokens } from './tokens'
import { ACCENT_INKS } from './accent-inks'

// Generate the CSS from the same definitions used by charts and the menu.
export default plugin(({ addBase }) => {
  addBase(
    Object.fromEntries(
      THEMES.map(theme => [
        `html[data-theme='${theme.id}']`,
        themeTokens(theme),
      ])
    )
  )
  addBase(
    Object.fromEntries(
      THEMES.flatMap(theme =>
        ACCENT_INKS.map(ink => [
          `html[data-theme='${theme.id}'][data-accent-ink='${ink.id}']`,
          accentInkTokens(theme, ink.id),
        ])
      )
    )
  )
})
