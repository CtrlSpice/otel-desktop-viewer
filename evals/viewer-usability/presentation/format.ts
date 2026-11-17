export const escapeHtml = (value: unknown) =>
  String(value).replace(
    /[&<>"']/g,
    character =>
      ({
        '&': '&amp;',
        '<': '&lt;',
        '>': '&gt;',
        '"': '&quot;',
        "'": '&#39;',
      })[character]!
  )

export const integer = (value: number | null) =>
  value === null ? 'Unknown' : value.toLocaleString('en-CA')

export const seconds = (value: number | null) =>
  value === null ? 'Unknown' : `${value.toFixed(1)} s`

export const cost = (value: number | null, coverage: string) =>
  value === null
    ? 'Unknown'
    : `C$${value.toFixed(3)}${coverage === 'partial' ? ' (partial)' : ''}`

export const percent = (value: number | null) =>
  value === null ? 'Unscored' : `${(value * 100).toFixed(1)}%`

export const runResult = (value: number) =>
  value === 0.5 ? '.5/1' : `${value}/1`

export const gradeLabel = (value: number) =>
  value === 1 ? 'complete' : value === 0.5 ? 'partial' : 'incorrect or missing'
