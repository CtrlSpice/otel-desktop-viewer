// Run before styles load so the first paint uses the selected system theme.
const theme = matchMedia('(prefers-color-scheme: light)')
const applyTheme = () => {
  document.documentElement.dataset.theme = theme.matches
    ? 'rose-pine-dawn'
    : 'rose-pine'
}
applyTheme()
theme.addEventListener('change', applyTheme)
