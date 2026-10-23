import { getContext, setContext } from 'svelte'
import type { WorkspaceProps } from '@/components/shared/WorkspaceShell.svelte'

const key = Symbol('workspace')

export function createWorkspaceContext() {
  let current = $state<(() => WorkspaceProps<unknown>) | null>(null)
  let owner: object | null = null
  const workspace = {
    get props() {
      return current?.() ?? null
    },
    register<T>(read: () => WorkspaceProps<T>) {
      // Erase only the list's item type. Its items, key function and renderer
      // always travel together; the shell never supplies an item of its own.
      const registration = read as () => WorkspaceProps<unknown>
      const token = {}
      owner = token
      current = registration
      return () => {
        // Teardown can run in the outgoing Svelte batch, whose reactive reads
        // still see the previous registration. Compare synchronous ownership.
        if (owner === token) {
          owner = null
          current = null
        }
      }
    },
  }
  setContext(key, workspace)
  return workspace
}

export function getWorkspaceContext() {
  return getContext<ReturnType<typeof createWorkspaceContext> | undefined>(key)
}
