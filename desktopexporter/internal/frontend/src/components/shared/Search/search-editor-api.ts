export type SearchEditorAPI = {
  submit: () => void
  clear: () => void
  /** Replace the visible query as one undoable edit and submit it. */
  setQuery?: (text: string) => void
}
