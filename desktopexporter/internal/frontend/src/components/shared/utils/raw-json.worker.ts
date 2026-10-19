import { formatRawJSON } from './raw-json-format'

self.onmessage = (event: MessageEvent<string>) => {
  self.postMessage(formatRawJSON(event.data))
}
