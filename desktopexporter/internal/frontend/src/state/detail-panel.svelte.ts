const STORAGE_KEY = 'signal-detail:open'

let open = $state(
  typeof localStorage === 'undefined' ||
    localStorage.getItem(STORAGE_KEY) !== 'false'
)
let revealRequest = $state(0)

function setOpen(value: boolean) {
  open = value
  if (typeof localStorage === 'undefined') return
  try {
    localStorage.setItem(STORAGE_KEY, String(open))
  } catch {
    // A full or blocked store costs persistence, not panel interaction.
  }
}

export const detailPanel = {
  get open() {
    return open
  },
  get revealRequest() {
    return revealRequest
  },
  setOpen,
  reveal() {
    setOpen(true)
    revealRequest++
  },
}
