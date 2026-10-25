import plugin from 'tailwindcss/plugin'
import { THEMES } from './index'
import { themeTokens } from './tokens'

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
})
