# Owned reads

This guide is for maintainers of the viewer's Svelte 5 frontend. Import the
read helpers from `@/services/reads`.

`Read<T>` owns one replaceable read. Its `data` is the last accepted value;
`pending` and `error` describe the requested work. Each load gets a numeric
`requestKey` unique across reads, so UI feedback can identify a new attempt.
Calling `load()` again on the same instance cancels its previous attempt and keeps
the accepted value visible. Successful results from superseded attempts are
discarded, even if the transport ignores cancellation. A failed replacement also
keeps the accepted value. Before the first success or after `clear()`, `data` is
`undefined`.

Create reads during component initialization. `createRead` and
`createReadMap` inherit the nearest `createReadGroup` context and dispose their
reads when the component unmounts. Create the group before its reads.
`createReadGroup` also disposes its group when the component unmounts.
Without a group, a read still owns its work and disposes on unmount, but does
not join a refresh group.

```ts
import { createReadGroup, createRead } from '@/services/reads'
import { telemetryAPI } from '@/services/telemetry-service'
import type { LogData } from '@/types/api-types'

let { selectedID }: { selectedID: string | null } = $props()
const group = createReadGroup(loadSelected)
const detail = createRead<LogData>()

function loadSelected() {
  const id = selectedID
  if (!id) {
    detail.clear()
    return
  }
  return detail.load(signal => telemetryAPI.getLog(id, signal))
}

$effect(() => {
  void loadSelected()
  return () => detail.cancel()
})
```

Here, `selectedID` is the requested log ID. Render the record and its Copy/Download
targets from `detail.data`. Keep identity and presentation metadata in the accepted
value when they cannot be derived from the record itself, such as the time window
used to fetch a list. A Refresh button can call `group.refresh()`; selection
changes call `loadSelected()` through the effect.

## Completion and cancellation

- `load(read, accept?, options?)` starts immediately, or after `options.delayMs`.
  Pending includes that delay. `accept(value)` receives a current successful result
  for side effects; its return value is ignored. The `Read` instance stores the result
  returned by the `read` callback in `data` after acceptance.
- `load` returns a completion promise for that attempt. `detail.completion`
  exposes the latest attempt's promise. Both resolve without a value when their
  attempt finishes or is cancelled, even if the transport ignores its abort signal.
  Read failures are stored in `error`.
- Pass `formatError`, `errorMessage` and `retainError` to `createRead`,
  `createReadMap` or the corresponding constructor. Pass `onError` in the third
  argument to `load`. Rejected values are normalized to `Error` before either error
  callback runs; `errorMessage` is the fallback for non-Error rejections.
  `formatError(error): string` supplies the displayed `error` text;
  `onError(error): void` runs side effects for a current failed attempt.
- `cancel()` aborts the owned signal and ends pending work while retaining data
  and errors. Calling it after settlement still aborts that attempt's signal.
  `clear()` also removes data and errors. `dispose()` cancels work and prevents future loads.
- Starting work clears its previous error unless `retainError` is enabled.
  A successful response clears the error in either case.
- The `read(signal, publish)` callback can publish intermediate results through
  `publish(() => { /* update received results */ })`. Late callbacks from cancelled
  attempts are ignored. This callback can update external state; it does not change
  `data`, which receives the final result. Pass the abort signal through every service call.

Use [`LoadingStatus`](../../components/shared/LoadingStatus.svelte) or
[`createLoadingNotice`](../../components/shared/utils/loading-notice.svelte.ts) to delay a loading notice until work
has been pending for 200 ms. A new request identity resets that delay:

```svelte
<script lang="ts">
  import LoadingStatus from '@/components/shared/LoadingStatus.svelte'
</script>

<LoadingStatus
  pending={detail.pending}
  requestKey={detail.requestKey}
  label="Loading log…"
/>
```

## Displayed records and refresh

Use separate reads when work has different lifetimes. A list replacement and
pagination need two reads: pagination for displayed list A can finish while
replacement B is pending or has failed. Cancel A's pagination in B's `accept`
callback, or when A is removed.

Use a `ReadMap` for multiple reads whose lifetimes follow different records.
For example, `logs.for(traceID)` returns the existing read for that trace's
logs or creates one. When the trace is no longer displayed or requested, call
`logs.delete(traceID)` to dispose its read. `map.clear()` disposes all entries;
`map.dispose()` also prevents later loads through the map. `map.requestKey`
combines its entries' attempt IDs for a loading notice shared by those reads.

`createReadGroup(callback)` registers the function that starts refresh reads.
`group.refresh()` waits for current reads registered with the group, runs that
callback, then waits for new reads in the group, including those started by Svelte
effects. Calls to
`group.refresh()` before the callback starts share the upcoming refresh. Calls
made after the callback starts queue one more refresh, which runs when the current
refresh's reads settle. Group disposal cancels its reads and discards queued
refreshes.

Pages use `const group = createPageReadGroup(callback)`, from
`@/contexts/page-read-group.svelte`, in place of `createReadGroup(callback)`.
It returns the group and subscribes it to database commit notifications while
the toolbar's Live toggle is enabled. The page disposes its group and removes the
Live subscription on unmount.
Both Live notifications and the page's Refresh action call `group.refresh()`.
Pass `onRefresh={() => void group.refresh()}` to
[`PageLayout`](../../components/shared/Layout/PageLayout.svelte) to render that action.
Use `createRead({ group: null })` or `createReadMap({ group: null })`
for independently scheduled work, such as navigation counts, freshness statistics
or an explicit Copy/Download action. Reads created this way still dispose on
component unmount.

Outside a component, use `new ReadGroup(callback)`, `new Read<T>({ group })`
and `new ReadMap<K, T>({ group })`. Dispose the group when the work's lifetime
ends. Without a group, dispose each read or map directly. These objects retain
only their own data; the library has no cross-route cache or automatic retries.
