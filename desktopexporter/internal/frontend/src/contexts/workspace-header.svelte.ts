import { getContext, setContext } from 'svelte'
import type { PaneHeaderProps } from '@/components/shared/PaneHeader.svelte'

const key = Symbol('workspace-main-header')

export function createWorkspaceHeader(fallback: () => PaneHeaderProps) {
  let read = $state<(() => PaneHeaderProps) | null>(null)
  let owner: object | null = null
  const header = {
    get props() {
      return read?.() ?? fallback()
    },
    register(next: () => PaneHeaderProps) {
      const token = {}
      owner = token
      read = next
      return () => {
        if (owner === token) {
          owner = null
          read = null
        }
      }
    },
  }
  setContext(key, header)
  return header
}

export type WorkspaceHeaderState = ReturnType<typeof createWorkspaceHeader>
export function getWorkspaceHeader() {
  return getContext<WorkspaceHeaderState | undefined>(key)
}
