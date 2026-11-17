export type ExportSignal = 'trace' | 'log' | 'metric'

async function fetchOTLP(
  url: string,
  abortSignal: AbortSignal
): Promise<Response> {
  const contentType = 'application/json'
  const response = await fetch(url, {
    signal: abortSignal,
    headers: { Accept: contentType },
  })
  if (!response.ok) {
    throw new Error(
      (await response.text()).trim() || `Export failed (${response.status})`
    )
  }
  if (
    response.headers.get('Content-Type')?.split(';')[0]?.trim() !== contentType
  ) {
    throw new Error('The viewer returned an unexpected export format')
  }
  return response
}

/** Preserve the exact JSON text, including 64-bit values and signed zero. */
export async function readOTLPText(
  signal: ExportSignal,
  id: string,
  abortSignal: AbortSignal
): Promise<string> {
  return readOTLPURL(
    `/export/${signal}s/${encodeURIComponent(id)}`,
    abortSignal
  )
}

/** Read complete server-produced text, never the bounded visual preview. */
export async function readOTLPURL(
  url: string,
  abortSignal: AbortSignal
): Promise<string> {
  const response = await fetchOTLP(url, abortSignal)
  const text = await response.text()
  abortSignal.throwIfAborted()
  return text
}

export async function downloadOTLP(
  signal: ExportSignal,
  id: string,
  abortSignal: AbortSignal
): Promise<void> {
  const response = await fetchOTLP(
    `/export/${signal}s/${encodeURIComponent(id)}`,
    abortSignal
  )
  // Keep response bytes intact: JSON.parse/stringify would turn -0 into 0.
  const blob = await response.blob()
  abortSignal.throwIfAborted()
  const objectURL = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = objectURL
  anchor.download = `${signal}-${id}.json`
  try {
    document.body.append(anchor)
    anchor.click()
  } finally {
    anchor.remove()
    URL.revokeObjectURL(objectURL)
  }
}
