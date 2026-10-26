export function openV2EventStream(url, { headers, deadlineSignal, cancellationSignal, stopSignal }) {
  // The same lifetime must cover both response headers and streamed bytes.
  return fetch(url + '/api/event', { headers,
    signal: AbortSignal.any([deadlineSignal, cancellationSignal, stopSignal].filter(Boolean)),
  })
}
