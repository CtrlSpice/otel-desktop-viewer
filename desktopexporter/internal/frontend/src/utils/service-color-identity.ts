import { sha256 } from '@noble/hashes/sha2.js'
import { bytesToHex } from '@noble/hashes/utils.js'

/** Keep oversized service names out of saved colour keys and summary payloads. */
export function serviceColorIdentity(name: string): string {
  const literal = () => (/^(sha256:|name:)/.test(name) ? `name:${name}` : name)
  if (name.length <= 75) return literal()
  const bytes = new TextEncoder().encode(name)
  return bytes.length <= 300 ? literal() : `sha256:${bytesToHex(sha256(bytes))}`
}

export const serviceColorStorageVersion = ':compact-service-names-v1'
