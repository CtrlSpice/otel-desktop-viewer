<script module lang="ts">
  // The kind is stored, so it survives a frontend that has not been taught
  // about a new one: fall back to the raw token rather than showing nothing.
  export function rejectionLabel(kind: string): string {
    switch (kind) {
      case 'span_already_stored':
        return 'duplicate span id'
      case 'span_refused':
        return 'rejected by the store'
      default:
        return kind
    }
  }
</script>

<script lang="ts">
  import { onMount, type Component, type Snippet } from 'svelte'
  import { HugeiconsIcon, type IconSvgElement } from '@hugeicons/svelte'
  import {
    BarChartHorizontalIcon,
    Cancel01Icon,
    ChartHistogramIcon,
    CheckmarkCircle02Icon,
    Copy01Icon,
  } from '@hugeicons/core-free-icons'
  import { LogIcon, QuestionMarkIcon } from '@/icons'
  import ReadonlyCodePanel from '@/components/shared/ReadonlyCodePanel.svelte'
  import FieldGroup from '@/components/shared/FieldGroup.svelte'
  import LogField from '@/components/shared/DetailField.svelte'
  import PageLayout from '@/components/shared/PageLayout.svelte'
  import IngestionIssues from '@/components/shared/IngestionIssues.svelte'
  import PaneHeader, {
    paneTabID,
    type PaneTab,
  } from '@/components/shared/PaneHeader.svelte'
  import { telemetryAPI } from '@/services/telemetry-service'
  import type { Stats } from '@/types/api-types'
  import type { ImportFailure } from '@/types/import-types'
  import luluImage from '@/assets/images/lulu-telescope-front.svg'

  let {
    importContent,
    importFailures = [],
  }: {
    importContent?: Snippet
    importFailures?: readonly ImportFailure[]
  } = $props()

  const POLL_INTERVAL_MS = 5000

  const OTLP_SNIPPETS = {
    http: `$ export OTEL_EXPORTER_OTLP_ENDPOINT="http://localhost:4318"
$ export OTEL_TRACES_EXPORTER="otlp"
$ export OTEL_EXPORTER_OTLP_PROTOCOL="http/protobuf"`,
    grpc: `$ export OTEL_EXPORTER_OTLP_ENDPOINT="http://localhost:4317"
$ export OTEL_TRACES_EXPORTER="otlp"
$ export OTEL_EXPORTER_OTLP_PROTOCOL="grpc"`,
  } as const

  type EndpointTab = keyof typeof OTLP_SNIPPETS

  function isEndpointTab(id: string): id is EndpointTab {
    return id in OTLP_SNIPPETS
  }

  const ENDPOINT_TABS: PaneTab[] = [
    { id: 'http', label: 'HTTP' },
    { id: 'grpc', label: 'gRPC' },
  ]
  const ENDPOINT_PANEL_ID = 'otlp-endpoint-panel'

  let stats = $state<Stats | null>(null)
  let endpointTab = $state<EndpointTab>('http')
  let endpointCopied = $state(false)

  async function copyEndpointSnippet() {
    try {
      await navigator.clipboard.writeText(OTLP_SNIPPETS[endpointTab])
      endpointCopied = true
      setTimeout(() => {
        endpointCopied = false
      }, 2000)
    } catch (e) {
      console.error('Failed to copy text:', e)
    }
  }

  async function fetchStats() {
    try {
      stats = await telemetryAPI.getStats()
    } catch (e) {
      console.error('Failed to fetch stats:', e)
    }
  }

  // Only ever a handful of entries -- the backend keys them by signal and
  // kind, so a replay loop that refuses thousands of spans is still one row.
  let rejections = $derived(stats?.rejections ?? [])

  function formatRelativeTime(timestampNs: bigint | null | undefined): string {
    if (timestampNs == null) return '-'
    const ms = Number(timestampNs / 1_000_000n)
    const seconds = Math.floor((Date.now() - ms) / 1000)
    if (seconds < 5) return 'just now'
    if (seconds < 60) return `${seconds}s ago`
    const minutes = Math.floor(seconds / 60)
    if (minutes < 60) return `${minutes}m ago`
    const hours = Math.floor(minutes / 60)
    if (hours < 24) return `${hours}h ago`
    return `${Math.floor(hours / 24)}d ago`
  }

  onMount(() => {
    fetchStats()
    const pollTimer = setInterval(fetchStats, POLL_INTERVAL_MS)
    return () => clearInterval(pollTimer)
  })
</script>

<div class="home-page">
  <PageLayout
    items={[]}
    emptyDrawer
    selectedID={null}
    drawerID="signal-drawer"
    drawerLabel="Home"
  >
    {#snippet drawerSearch()}
      <div class="flex w-full min-w-0 items-center gap-1">
        <input
          class="input typed-field typed-field--pill min-w-0 flex-1"
          type="search"
          aria-label="Search"
          placeholder="Search…"
          readonly
          aria-disabled="true"
        />
        <button
          type="button"
          class="btn btn-circle btn-ghost btn-sm shrink-0 text-sm text-base-content"
          aria-label="Clear search"
          disabled
        >
          <HugeiconsIcon
            icon={Cancel01Icon}
            size={17}
            strokeWidth={1.5}
            aria-hidden="true"
          />
        </button>
        <button
          type="button"
          class="btn btn-circle btn-ghost btn-sm shrink-0 text-sm text-base-content"
          aria-label="Search query help"
          disabled
        >
          <QuestionMarkIcon class="size-4" />
        </button>
      </div>
    {/snippet}

    {#snippet main()}
      <div class="home-page__main">
        <header class="mb-6">
          <div
            class="flex flex-col items-center gap-8 text-center min-[1100px]:flex-row min-[1100px]:items-center min-[1100px]:gap-10 min-[1100px]:text-left"
          >
            <img
              src={luluImage}
              alt="Lulu the pink pixel-art axolotl looking through her telescope."
              class="h-48 w-48 shrink-0 object-contain min-[1100px]:h-64 min-[1100px]:w-64"
            />
            <div class="min-w-0 py-2">
              <p
                class="home-hero__eyebrow mb-2 text-xs font-semibold uppercase tracking-[0.12em]"
              >
                Local-first telemetry
              </p>
              <h1
                class="mb-3 text-xl font-normal leading-tight tracking-tight text-base-content"
              >
                OpenTelemetry Desktop Viewer
              </h1>
              <p class="home-hero__lede max-w-xl text-base leading-relaxed">
                Collect, visualize, and query your OpenTelemetry data locally —
                without shipping it to the cloud.
              </p>
            </div>
          </div>
        </header>

        {@render importContent?.()}

        <section class="section home-endpoint-section">
          <h2 class="section-title">Configure your OTLP exporter</h2>
          <p class="section-description">
            Point your OpenTelemetry SDK, collector, or agent at this viewer
            with the OTLP exporter settings below. Copy the variables for your
            protocol, then paste them into a terminal session, a <code
              class="home-inline-code">.env</code
            >
            file, or wherever you usually set environment variables for the process
            that exports telemetry.
          </p>
          <div class="home-endpoint-chrome">
            <PaneHeader
              mode="tabs"
              tabs={ENDPOINT_TABS}
              activeID={endpointTab}
              onSelect={id => {
                if (isEndpointTab(id)) endpointTab = id
              }}
              ariaLabel="OTLP endpoint protocol"
              tabPanelID={ENDPOINT_PANEL_ID}
            >
              {#snippet right()}
                <button
                  type="button"
                  class="drawer-header-btn tooltip tooltip-left"
                  onclick={copyEndpointSnippet}
                  data-tip={endpointCopied ? 'Copied!' : 'Copy snippet'}
                  aria-label={endpointCopied ? 'Copied' : 'Copy snippet'}
                >
                  {#if endpointCopied}
                    <HugeiconsIcon
                      icon={CheckmarkCircle02Icon}
                      size="1em"
                      strokeWidth={1.5}
                      class="h-4 w-4 shrink-0"
                      aria-hidden="true"
                    />
                  {:else}
                    <HugeiconsIcon
                      icon={Copy01Icon}
                      size="1em"
                      strokeWidth={1.5}
                      class="h-4 w-4 shrink-0"
                      aria-hidden="true"
                    />
                  {/if}
                </button>
              {/snippet}
            </PaneHeader>
            <div
              class="home-endpoint-chrome__body"
              id={ENDPOINT_PANEL_ID}
              role="tabpanel"
              aria-labelledby={paneTabID(ENDPOINT_PANEL_ID, endpointTab)}
              tabindex="0"
            >
              <ReadonlyCodePanel
                code={OTLP_SNIPPETS[endpointTab]}
                ariaLabel="OTLP exporter environment variables"
                embedded
              />
            </div>
          </div>
        </section>

        <section class="section">
          <h2 class="section-title">Example with otel-cli</h2>
          <p class="section-description">
            If you have
            <a
              href="https://github.com/equinix-labs/otel-cli"
              class="external-link"
              target="_blank"
              rel="noopener noreferrer"
            >
              otel-cli
            </a>
            installed, you can send example data:
          </p>
          <ReadonlyCodePanel
            ariaLabel="otel-cli example commands"
            code={`# start the desktop viewer
$ otel-desktop-viewer

# configure otel-cli
$ export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318

# generate spans!
$ otel-cli exec --service my-service --name "curl google" curl https://google.com`}
          />
        </section>
      </div>
    {/snippet}

    {#snippet detail()}
      <div class="home-page__detail">
        <PaneHeader mode="title" title="Overview" ariaLabel="Overview" />

        <div class="home-page__detail-scroll">
          {#snippet signalOverviewLink(
            href: string,
            label: string,
            icon: IconSvgElement | null,
            CustomIcon: Component | null
          )}
            <a
              {href}
              class="home-summary__icon-link"
              aria-label="View all {label.toLowerCase()}"
            >
              {#if icon}
                <HugeiconsIcon
                  {icon}
                  size="1em"
                  strokeWidth={1.5}
                  class="h-4 w-4 shrink-0"
                  aria-hidden="true"
                />
              {:else if CustomIcon}
                <CustomIcon class="h-4 w-4 shrink-0" aria-hidden="true" />
              {/if}
            </a>
          {/snippet}

          {#snippet signalOverviewHeader(label: string, count: number)}
            <div class="home-summary__header">
              <span class="home-summary__title">{label}</span>
              <span class="badge-count home-summary__badge"
                >{count} {label.toLowerCase()}</span
              >
            </div>
          {/snippet}

          <div class="home-summary">
            {@render signalOverviewLink(
              '/traces',
              'Traces',
              BarChartHorizontalIcon,
              null
            )}
            <FieldGroup label="Traces">
              {#snippet headerAction()}
                {@render signalOverviewHeader(
                  'Traces',
                  stats?.traces.traceCount ?? 0
                )}
              {/snippet}
              <table class="detail-fields w-full" aria-label="Traces overview">
                <tbody>
                  <LogField
                    fieldName="spans"
                    fieldType="uint32"
                    showType={false}
                    fieldValue={String(stats?.traces.spanCount ?? 0)}
                  />
                  <LogField
                    fieldName="services"
                    fieldType="uint32"
                    showType={false}
                    fieldValue={String(stats?.traces.serviceCount ?? 0)}
                  />
                  <LogField
                    fieldName="errors"
                    fieldType="uint32"
                    showType={false}
                    fieldValue={String(stats?.traces.errorCount ?? 0)}
                  />
                  <LogField
                    fieldName="last received"
                    fieldType="string"
                    showType={false}
                    fieldValue={formatRelativeTime(
                      stats?.traces.lastReceived ?? null
                    )}
                  />
                </tbody>
              </table>
            </FieldGroup>
          </div>

          <div class="home-summary">
            {@render signalOverviewLink(
              '/metrics',
              'Metrics',
              ChartHistogramIcon,
              null
            )}
            <FieldGroup label="Metrics">
              {#snippet headerAction()}
                {@render signalOverviewHeader(
                  'Metrics',
                  stats?.metrics.metricCount ?? 0
                )}
              {/snippet}
              <table class="detail-fields w-full" aria-label="Metrics overview">
                <tbody>
                  <LogField
                    fieldName="data points"
                    fieldType="uint32"
                    showType={false}
                    fieldValue={String(stats?.metrics.dataPointCount ?? 0)}
                  />
                  <LogField
                    fieldName="last received"
                    fieldType="string"
                    showType={false}
                    fieldValue={formatRelativeTime(
                      stats?.metrics.lastReceived ?? null
                    )}
                  />
                </tbody>
              </table>
            </FieldGroup>
          </div>

          <div class="home-summary">
            {@render signalOverviewLink('/logs', 'Logs', null, LogIcon)}
            <FieldGroup label="Logs">
              {#snippet headerAction()}
                {@render signalOverviewHeader(
                  'Logs',
                  stats?.logs.logCount ?? 0
                )}
              {/snippet}
              <table class="detail-fields w-full" aria-label="Logs overview">
                <tbody>
                  <LogField
                    fieldName="errors"
                    fieldType="uint32"
                    showType={false}
                    fieldValue={String(stats?.logs.errorCount ?? 0)}
                  />
                  <LogField
                    fieldName="last received"
                    fieldType="string"
                    showType={false}
                    fieldValue={formatRelativeTime(
                      stats?.logs.lastReceived ?? null
                    )}
                  />
                </tbody>
              </table>
            </FieldGroup>
          </div>

          <IngestionIssues
            {rejections}
            {importFailures}
            {rejectionLabel}
            {formatRelativeTime}
          />
        </div>

        {#if rejections.length === 0 && importFailures.length === 0}
          <p class="home-detail__coming-soon">more coming soon…</p>
        {/if}
      </div>
    {/snippet}

    {#snippet pageFooter()}
      <div class="home-page__footer">
        <p class="home-page__footer-text">
          <a
            href="https://github.com/CtrlSpice/lulu"
            class="home-page__footer-link"
            target="_blank"
            rel="noopener noreferrer"
          >
            Pixel Lulu
          </a>
          · Original painting by
          <a
            href="https://cbatesonart.artstation.com/"
            class="home-page__footer-link"
            target="_blank"
            rel="noopener noreferrer"
          >
            Chelsey Bateson
          </a>
        </p>
      </div>
    {/snippet}
  </PageLayout>
</div>

<style lang="postcss">
  @reference "../app.css";

  .home-page {
    @apply flex min-h-0 min-w-0 flex-1 flex-col;
    --home-secondary-text: var(--color-subtle);
    --home-accent-text: var(--color-primary);
  }

  /* Keep Dawn's Rosé Pine hierarchy, but use darker members of the same
     purple family where this page renders small text on pale panels. */
  :global(html[data-theme='rose-pine-dawn']) .home-page {
    --home-secondary-text: var(--color-base-content);
    --home-accent-text: #6e577f;
  }

  .home-page__main {
    @apply min-h-0 flex-1 overflow-y-auto px-4 py-4 min-[900px]:px-6;
  }

  .home-page__detail {
    @apply flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden text-sm;
  }

  .home-page__detail-scroll {
    @apply min-h-0 flex-1 overflow-y-auto py-2;
    scrollbar-width: thin;
  }

  .home-detail__coming-soon {
    @apply shrink-0 px-3 pb-3 pt-1 text-center text-sm italic;
    color: var(--home-secondary-text);
  }

  .home-summary {
    @apply relative;
  }

  .home-summary :global(.field-group__heading) {
    /* Reserve the original 32px icon and 8px gap outside the disclosure. */
    @apply min-h-11 pl-13;
  }

  .home-summary__header {
    @apply flex min-w-0 flex-1 items-center gap-2;
  }

  .home-summary__icon-link {
    @apply absolute left-3 top-1.5;
    @apply btn btn-soft btn-primary btn-sm btn-circle h-8 w-8 min-h-8 shrink-0 border-transparent bg-primary/10 text-primary no-underline shadow-none;
  }

  .home-summary__icon-link:hover {
    @apply border-transparent bg-primary/15 text-primary;
  }

  .home-summary__title {
    @apply truncate text-sm font-medium;
    color: var(--home-secondary-text);
  }

  .home-summary__badge {
    @apply ml-auto shrink-0;
  }

  .section {
    @apply mb-5;
  }

  .section-title {
    @apply mb-3 text-base font-normal tracking-tight text-base-content;
  }

  .home-hero__eyebrow {
    color: var(--home-accent-text);
  }

  .home-hero__lede,
  .section-description {
    color: var(--home-secondary-text);
  }

  .section-description {
    @apply mb-4 max-w-2xl leading-relaxed;
  }

  .home-inline-code {
    @apply rounded bg-base-100 px-1 py-0.5 font-mono text-base;
    color: var(--home-secondary-text);
  }

  .home-endpoint-section .section-title {
    @apply mb-2;
  }

  .home-endpoint-section .section-description {
    @apply max-w-none;
  }

  .home-endpoint-chrome {
    @apply overflow-hidden rounded-xl border border-base-300 bg-base-200;
  }

  .home-endpoint-chrome__body {
    @apply bg-base-100;
  }

  .external-link {
    @apply link;
    color: var(--home-accent-text);
  }

  .home-page__detail :global(.detail-cell__key),
  .home-page__detail :global(.detail-cell__type) {
    color: var(--home-secondary-text);
  }

  .home-page__footer {
    @apply flex min-h-[var(--app-footer-height)] shrink-0 items-center justify-center bg-base-100 px-4 py-1.5;
  }

  .home-page__footer-text {
    @apply text-center text-sm;
    color: var(--home-secondary-text);
  }

  .home-page__footer-link {
    @apply link;
    color: var(--home-accent-text);
  }
</style>
