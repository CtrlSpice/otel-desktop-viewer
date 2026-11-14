# Owned reads

This guide is for maintainers of the viewer's Svelte 5 frontend. Import the
resource helpers from `@/services/resources`.

`Resource<T>` owns one replaceable read. Its `data` is the last accepted value;
`pending` and `error` describe the requested work. Each load gets a numeric
`requestKey` unique across resources, so UI feedback can identify a new attempt.
Starting another read cancels the previous attempt and keeps the accepted value
visible. A failed replacement also keeps that value. Before the first success or
after `clear()`, `data` is `undefined`.

Create resources during component initialization. `createResource` and
`createResourceMap` inherit the nearest `createReadScope` context and dispose their
resources when the component unmounts. Create the scope before its resources.
Without a scope, a resource still owns its work and disposes on unmount, but does
not join a refresh group.

```ts
import { createReadScope, createResource } from '@/services/resources'
import { telemetryAPI } from '@/services/telemetry-service'
import type { LogData } from '@/types/api-types'

let { selectedID }: { selectedID: string | null } = $props()
const group = createReadScope(loadSelected)
const detail = createResource<LogData>()

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
  for side effects; its return value is ignored. The resource stores the result
  returned by `read` in `data` after acceptance.
- `load` returns a completion promise for that attempt. `resource.completion`
  exposes the latest attempt's promise. Both resolve without a value when their
  attempt finishes or is cancelled, even if the transport ignores its abort signal.
  Read failures are stored in `error`.
- Pass `formatError`, `errorMessage` and `retainError` to `createResource`,
  `createResourceMap` or the corresponding constructor. Pass `onError` in the third
  argument to `load`. Rejected values are normalized to `Error` before either error
  callback runs; `errorMessage` is the fallback for non-Error rejections.
  `formatError(error): string` supplies the displayed `error` text;
  `onError(error): void` runs side effects for a current failed attempt.
- `cancel()` aborts the owned signal and ends pending work while retaining data
  and errors. Calling it after settlement still aborts that attempt's signal.
  `clear()` also removes data and errors. `dispose()` cancels work and prevents future loads.
- Starting work clears its previous error unless `retainError` is enabled.
  A successful response clears the error in either case.
- A read with intermediate results can use its second argument,
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

## Displayed owners and refresh

Use separate resources when work has different lifetimes. A list replacement and
pagination need two resources: pagination for displayed list A can finish while
replacement B is pending or has failed. Cancel A's pagination when B is accepted
or A is removed.

Use a resource map for multiple reads whose lifetimes follow different records.
For example, `logs.for(traceID)` returns the existing resource for that trace's
logs or creates one. When the trace is no longer displayed or requested, call
`logs.delete(traceID)` to dispose its resource. `map.clear()` disposes all entries;
`map.dispose()` also prevents later loads through the map. `map.requestKey`
combines its entries' attempt IDs for a loading notice shared by those reads.

`createReadScope(callback)` registers the function that starts refresh reads.
`scope.refresh()` waits for current reads, runs that callback, then waits for the
new reads and any dependent reads started by Svelte effects. Calls to
`scope.refresh()` before the callback starts share the upcoming refresh. Calls
made after the callback starts queue one more refresh, which runs when the current
refresh's reads settle. Scope disposal cancels its resources and discards queued
refreshes.

Pages use `const scope = createPageResources(callback)`, from
`@/contexts/page-resources.svelte`, in place of `createReadScope(callback)`.
It returns the scope and subscribes it to database commit notifications while
the toolbar's Live toggle is enabled.
Both Live notifications and the page's Refresh action call `scope.refresh()`.
Pass `onRefresh={() => void scope.refresh()}` to
[`PageLayout`](../../components/shared/PageLayout.svelte) to render that action.
Use `createResource({ scope: null })` or `createResourceMap({ scope: null })`
for independently scheduled work, such as navigation counts, freshness statistics
or an explicit Copy/Download action. Resources created this way still dispose on
component unmount.

Outside a component, use `new ReadScope(callback)`, `new Resource<T>({ scope })`
and `new ResourceMap<K, T>({ scope })`. Dispose the scope when the work's lifetime
ends. Without a scope, dispose each resource or map directly. These objects retain
only their own data; the library has no cross-route cache or automatic retries.
