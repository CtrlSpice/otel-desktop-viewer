import {
  arrowStreamContentType,
  decodeArrow,
  type ArrowFrame,
} from './arrow-data'
import { JsonRpcError } from './service-error'

/** JSON parameters in, complete binary IPC out. Aborting fetch cancels DuckDB. */
export async function fetchArrow(
  method: string,
  parameters: string,
  signal?: AbortSignal
): Promise<ArrowFrame> {
  signal?.throwIfAborted()
  const response = await fetch(`/api/data/${encodeURIComponent(method)}`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Accept: arrowStreamContentType,
    },
    body: parameters,
    signal,
  })
  const contentType = response.headers
    .get('Content-Type')
    ?.split(';')[0]
    ?.trim()
  if (!response.ok) {
    if (contentType === 'application/json') {
      const error: { code: number; message: string } = await response.json()
      signal?.throwIfAborted()
      throw new JsonRpcError(error.code, error.message)
    }
    throw new Error(`HTTP error! status: ${response.status}`)
  }
  if (contentType !== arrowStreamContentType)
    throw new Error('Expected an Arrow dataset response')
  const rows = response.headers.get('X-Arrow-Rows')
  if (rows === null || !/^\d+$/.test(rows))
    throw new Error('Arrow response is missing its row count')
  const bytes = new Uint8Array(await response.arrayBuffer())
  signal?.throwIfAborted()
  const frame = decodeArrow(bytes, BigInt(rows))
  signal?.throwIfAborted()
  return frame
}
