export type ExportSignal = 'trace' | 'log' | 'metric'

export async function downloadOTLP(
  signal: ExportSignal,
  id: string,
  abortSignal: AbortSignal
): Promise<void> {
  const contentType = 'application/json'
  const response = await fetch(`/export/${signal}s/${encodeURIComponent(id)}`, {
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
