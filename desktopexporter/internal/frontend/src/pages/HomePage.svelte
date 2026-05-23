<script lang="ts">
  import { onMount, type Component } from 'svelte'
  import {
    BarChartHorizontalIcon,
    ChartHistogramIcon,
    CheckmarkCircleIcon,
    CopyIcon,
    LogIcon,
  } from '@/icons'
  import ReadonlyCodePanel from '@/components/shared/ReadonlyCodePanel.svelte'
  import FieldGroup from '@/components/shared/FieldGroup.svelte'
  import LogField from '@/components/logs/LogField.svelte'
  import PageLayout from '@/components/shared/PageLayout.svelte'
  import PaneHeader, { type PaneTab } from '@/components/shared/PaneHeader.svelte'
  import { telemetryAPI } from '@/services/telemetry-service'
  import type { Stats } from '@/types/api-types'
  import luluImage from '@/assets/images/lulu.png'
  import axolotlImage from '@/assets/images/axolotl.svg'

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
    selectedId={null}
    drawerId="signal-drawer"
    drawerLabel="Home"
    resizableStorageKey="home-panels"
    defaultMainWidth={0.58}
    minDetailPx={280}
  >
    {#snippet main()}
      <div class="home-page__main">
        <header class="mb-6">
          <div
            class="flex flex-col items-center gap-8 text-center min-[1100px]:flex-row min-[1100px]:items-center min-[1100px]:gap-10 min-[1100px]:text-left"
          >
            <img
              src={luluImage}
              alt="A pink axolotl is striking a heroic pose while gazing at a field of stars through a telescope. Her name is Lulu Axol'Otel the First, valiant adventurer and observability queen."
              class="h-48 w-48 shrink-0 object-contain drop-shadow-sm min-[1100px]:h-64 min-[1100px]:w-64"
            />
            <div class="min-w-0 py-2">
              <p
                class="mb-2 text-xs font-semibold uppercase tracking-[0.12em] text-primary/80"
              >
                Local-first telemetry
              </p>
              <h1
                class="mb-3 text-base font-semibold leading-tight tracking-tight text-base-content"
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

        <section class="section home-endpoint-section">
          <h2 class="section-title">Configure your OTLP exporter</h2>
          <p class="section-description">
            Point your OpenTelemetry SDK, collector, or agent at this viewer with
            the OTLP exporter settings below. Copy the variables for your protocol,
            then paste them into a terminal session, a <code class="home-inline-code">.env</code>
            file, or wherever you usually set environment variables for the process
            that exports telemetry.
          </p>
          <div class="home-endpoint-chrome">
            <PaneHeader
              mode="tabs"
              tabs={ENDPOINT_TABS}
              activeId={endpointTab}
              onSelect={id => {
                if (isEndpointTab(id)) endpointTab = id
              }}
              ariaLabel="OTLP endpoint protocol"
            >
              {#snippet right()}
                <button
                  type="button"
                  class="drawer-header-btn"
                  onclick={copyEndpointSnippet}
                  title={endpointCopied ? 'Copied!' : 'Copy snippet'}
                  aria-label={endpointCopied ? 'Copied' : 'Copy snippet'}
                >
                  {#if endpointCopied}
                    <CheckmarkCircleIcon class="h-4 w-4 shrink-0" aria-hidden="true" />
                  {:else}
                    <CopyIcon class="h-4 w-4 shrink-0" aria-hidden="true" />
                  {/if}
                </button>
              {/snippet}
            </PaneHeader>
            <div class="home-endpoint-chrome__body">
              <ReadonlyCodePanel code={OTLP_SNIPPETS[endpointTab]} embedded />
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
        {#snippet signalOverviewHeader(
          href: string,
          label: string,
          Icon: Component,
          count: number
        )}
          <div class="home-summary__header">
            <a
              {href}
              class="home-summary__icon-link"
              aria-label="View all {label.toLowerCase()}"
            >
              <Icon class="h-4 w-4 shrink-0" aria-hidden="true" />
            </a>
            <span class="home-summary__title">{label}</span>
            <span class="badge-count home-summary__badge">{count}</span>
          </div>
        {/snippet}

        <FieldGroup label="Traces">
          {#snippet headerAction()}
            {@render signalOverviewHeader(
              '/traces',
              'Traces',
              BarChartHorizontalIcon,
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
                fieldValue={formatRelativeTime(stats?.traces.lastReceived ?? null)}
              />
            </tbody>
          </table>
        </FieldGroup>

        <FieldGroup label="Metrics">
          {#snippet headerAction()}
            {@render signalOverviewHeader(
              '/metrics',
              'Metrics',
              ChartHistogramIcon,
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
                fieldValue={formatRelativeTime(stats?.metrics.lastReceived ?? null)}
              />
            </tbody>
          </table>
        </FieldGroup>

        <FieldGroup label="Logs">
          {#snippet headerAction()}
            {@render signalOverviewHeader(
              '/logs',
              'Logs',
              LogIcon,
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
                fieldValue={formatRelativeTime(stats?.logs.lastReceived ?? null)}
              />
            </tbody>
          </table>
        </FieldGroup>
        </div>

        <p class="home-detail__coming-soon">more coming soon…</p>
      </div>
    {/snippet}

    {#snippet pageFooter()}
      <div class="home-page__footer">
        <p class="home-page__footer-text">
          Made with
          <img src={axolotlImage} alt="axolotl emoji" class="home-page__footer-icon" />
          by
          <a
            href="https://github.com/CtrlSpice"
            class="home-page__footer-link"
            target="_blank"
            rel="noopener noreferrer"
          >
            Mila Ardath
          </a>
          , with Artwork by
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
    color: var(--color-muted);
  }

  .home-summary__header {
    @apply flex min-w-0 flex-1 items-center gap-2;
  }

  .home-summary__icon-link {
    @apply btn btn-soft btn-primary btn-sm btn-circle h-8 w-8 min-h-8 shrink-0 border-transparent bg-primary/10 text-primary no-underline shadow-none;
  }

  .home-summary__icon-link:hover {
    @apply border-transparent bg-primary/15 text-primary;
  }

  .home-summary__title {
    @apply truncate text-sm font-medium;
    color: var(--color-subtle);
  }

  .home-summary__badge {
    @apply ml-auto shrink-0;
  }

  .section {
    @apply mb-5;
  }

  .section-title {
    @apply mb-3 text-base font-semibold tracking-tight text-base-content;
  }

  .home-hero__lede,
  .section-description {
    color: var(--color-subtle);
  }

  .section-description {
    @apply mb-4 max-w-2xl leading-relaxed;
  }

  .home-inline-code {
    @apply rounded bg-base-300/60 px-1 py-0.5 font-mono text-[0.9em] text-base-content/80;
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

  .home-endpoint-chrome :global(.pane-header__tab.tab-active) {
    --tab-bg: var(--color-base-100);
  }

  .home-endpoint-chrome__body {
    @apply bg-base-100;
  }

  .external-link {
    @apply link link-primary;
  }

  .home-page__footer {
    @apply flex min-h-[var(--app-footer-height)] shrink-0 items-center justify-center bg-base-100 px-4;
  }

  .home-page__footer-text {
    @apply text-center text-sm text-base-content/60;
  }

  .home-page__footer-icon {
    @apply mx-0.5 inline-block h-[1em] w-[1em] max-h-3.5 max-w-3.5 align-middle object-contain;
  }

  .home-page__footer-link {
    @apply link link-primary;
  }
</style>
