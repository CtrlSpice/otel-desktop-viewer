import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'

interface RawLockPackage {
  version?: string
  integrity?: string
  optional?: boolean
}

interface RawLockfile {
  lockfileVersion?: number
  packages?: Record<string, RawLockPackage>
}

interface LockPackage {
  version: string | undefined
  integrity: string | undefined
  optional: boolean
}

interface Lockfile {
  lockfileVersion: number | undefined
  packages: Map<string, LockPackage>
}

function isRawLockfile(value: RawLockfile | null): value is RawLockfile {
  return value !== null && typeof value === 'object'
}

function isErrnoException(error: Error): error is NodeJS.ErrnoException {
  return 'code' in error
}

function parseLockfile(raw: string, path: string): Lockfile {
  const parsed: RawLockfile | null = JSON.parse(raw)
  if (!isRawLockfile(parsed)) {
    throw new Error(`Expected JSON object in ${path}`)
  }

  const packages = new Map<string, LockPackage>()
  if (parsed.packages) {
    for (const [pkg, entry] of Object.entries(parsed.packages)) {
      if (pkg === '') continue
      packages.set(pkg, {
        version: entry.version,
        integrity: entry.integrity,
        optional: entry.optional === true,
      })
    }
  }

  return {
    lockfileVersion: parsed.lockfileVersion,
    packages,
  }
}

const frontend = fileURLToPath(new URL('../', import.meta.url))
const lockPath = join(frontend, 'package-lock.json')
const installedPath = join(frontend, 'node_modules', '.package-lock.json')

const expected = parseLockfile(readFileSync(lockPath, 'utf8'), lockPath)

let installed: Lockfile
try {
  installed = parseLockfile(readFileSync(installedPath, 'utf8'), installedPath)
} catch (error) {
  if (
    error instanceof Error &&
    isErrnoException(error) &&
    error.code === 'ENOENT'
  ) {
    console.error(
      "frontend node_modules is missing; run 'make install-clean' first."
    )
    process.exit(1)
  }
  throw error
}

const mismatches: string[] = []
if (expected.lockfileVersion !== installed.lockfileVersion) {
  mismatches.push(
    `lockfileVersion: lockfile ${expected.lockfileVersion}, installed ${installed.lockfileVersion}`
  )
}

for (const [pkg, inst] of installed.packages) {
  const exp = expected.packages.get(pkg)
  if (!exp) {
    mismatches.push(`${pkg}: installed (${inst.version}), absent from lockfile`)
    continue
  }
  if (exp.version !== inst.version || exp.integrity !== inst.integrity) {
    mismatches.push(
      `${pkg}: lockfile ${exp.version}, installed ${inst.version}`
    )
  }
}

for (const [pkg, exp] of expected.packages) {
  if (exp.optional) continue
  if (!installed.packages.has(pkg)) {
    mismatches.push(
      `${pkg}: missing from node_modules (lockfile ${exp.version})`
    )
  }
}

if (mismatches.length > 0) {
  console.error('frontend node_modules is out of sync with package-lock.json:')
  for (const line of mismatches.slice(0, 5)) {
    console.error(`  - ${line}`)
  }
  if (mismatches.length > 5) {
    console.error(`  ... and ${mismatches.length - 5} more`)
  }
  console.error(
    "Run 'make install-clean' so embedded frontend builds match CI asset hashes."
  )
  process.exit(1)
}
