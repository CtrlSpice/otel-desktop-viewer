import fs from 'node:fs'
import path from 'node:path'

const inside = (directory, file) => file === directory || file.startsWith(directory + path.sep)

// New files are checked through their nearest existing parent. Existing files
// are resolved through symlinks before the native permission decision is used.
export function workspacePath(workspace, value, mutation = false) {
  if (typeof value !== 'string' || !value) throw new Error('Missing evaluation workspace path')
  const root = fs.realpathSync(workspace), requested = path.resolve(root, value)
  let existing = requested
  while (!fs.existsSync(existing)) {
    const parent = path.dirname(existing)
    if (parent === existing) throw new Error('Evaluation workspace path has no existing parent')
    existing = parent
  }
  const canonical = path.resolve(fs.realpathSync(existing), path.relative(existing, requested))
  if (!inside(root, requested) || !inside(root, canonical)) throw new Error('Evaluation workspace boundary: external paths and symlinks are blocked')
  if (mutation && (inside(path.join(root, 'bin'), requested) || inside(path.join(root, 'bin'), canonical)))
    throw new Error('The evaluated viewer executable is read-only')
  return canonical
}

export function workspaceSearch(workspace, value = '.') {
  const root = workspacePath(workspace, value)
  const visit = file => {
    workspacePath(workspace, file)
    const stat = fs.lstatSync(file)
    // Do not follow links during a recursive search, including links back into
    // the workspace. This also avoids directory cycles in model-created files.
    if (stat.isSymbolicLink()) throw new Error('Evaluation search cannot traverse symbolic links')
    if (stat.isDirectory()) for (const entry of fs.readdirSync(file)) visit(path.join(file, entry))
  }
  visit(root)
  return root
}
