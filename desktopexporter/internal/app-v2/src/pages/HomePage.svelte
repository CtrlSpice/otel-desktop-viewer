<!-- HomePage.svelte - The welcome/landing page -->
<div class="flex flex-col items-center justify-center w-full overflow-y-auto">
  <div class="card bg-base-100 shadow-2xl my-16 min-w-[960px] p-8 w-4/5">
    <!-- Header with Lulu and Welcome -->
    <div class="card-header pb-0">
      <div class="flex w-full">
        <img
          src="/src/assets/images/lulu.png"
          alt="A pink axolotl is striking a heroic pose while gazing at a field of stars through a telescope. Her name is Lulu Axol'Otel the First, valiant adventurer and observability queen."
          class="max-h-96 max-w-96 rounded-lg"
        />
        <div class="ml-6 flex flex-col justify-end">
          <div class="space-y-3">
            <h1 class="text-2xl font-bold">
              Welcome to the OpenTelemetry Desktop Viewer.
            </h1>
            <div class="divider"></div>
            <p class="text-base-content/80">
              This CLI tool allows you to receive OpenTelemetry traces while
              working on your local machine, helping you visualize and explore
              your trace data without needing to send it on to a telemetry
              vendor.
            </p>
          </div>
          <div class="space-y-3 mt-6">
            <h2 class="text-xl font-semibold">Explore with Sample Data</h2>
            <div class="divider"></div>
            <p class="text-base-content/80">
              If you would like to explore the application without sending it
              anything, you can do so by loading some sample data.
            </p>
            <button class="btn btn-primary w-fit"> Load Sample Data </button>
          </div>
        </div>
      </div>
    </div>

    <!-- Configuration Instructions -->
    <div class="card-body">
      <div class="space-y-3">
        <h2 class="text-xl font-semibold">
          Configuring your OpenTelemetry SDK
        </h2>
        <div class="divider"></div>
        <p class="text-base-content/80">
          To send telemetry to OpenTelemetry Desktop Viewer from your
          application, you need to configure an OTLP exporter to send via grpc
          to <code class="bg-base-200 px-2 py-1 rounded"
            >http://localhost:4317</code
          >
          or via http to
          <code class="bg-base-200 px-2 py-1 rounded"
            >http://localhost:4318</code
          >.
        </p>
        <p class="text-base-content/80">
          If your OpenTelemetry SDK OTLP exporter supports
          <a
            href="https://opentelemetry.io/docs/concepts/sdk-configuration/otlp-exporter-configuration/"
            class="link link-primary"
            target="_blank"
            rel="noopener noreferrer"
          >
            configuration via environment variables
          </a>
          then you should be able to send to
          <code class="bg-base-200 px-2 py-1 rounded">otel-desktop-viewer</code>
          with the following environment variables set.
        </p>

        <h3 class="text-lg font-medium">For HTTP:</h3>
        <div class="code-block">
          <pre><code
              ><span class="prompt">$</span
              > export OTEL_EXPORTER_OTLP_ENDPOINT="http://localhost:4318"
<span class="prompt">$</span> export OTEL_TRACES_EXPORTER="otlp"
<span class="prompt">$</span
              > export OTEL_EXPORTER_OTLP_PROTOCOL="http/protobuf"</code
            ></pre>
        </div>

        <h3 class="text-lg font-medium">For GRPC:</h3>
        <div class="code-block">
          <pre><code
              ><span class="prompt">$</span
              > export OTEL_EXPORTER_OTLP_ENDPOINT="http://localhost:4317"
<span class="prompt">$</span> export OTEL_TRACES_EXPORTER="otlp"
<span class="prompt">$</span> export OTEL_EXPORTER_OTLP_PROTOCOL="grpc"</code
            ></pre>
        </div>
      </div>

      <!-- otel-cli Example -->
      <div class="space-y-3 mt-8">
        <h2 class="text-xl font-semibold">Example with otel-cli</h2>
        <div class="divider"></div>
        <p class="text-base-content/80">
          If you have
          <a
            href="https://github.com/equinix-labs/otel-cli"
            class="link link-primary"
            target="_blank"
            rel="noopener noreferrer"
          >
            otel-cli
          </a>
          installed, you can send some example data with the following script.
        </p>
        <div class="code-block">
          <pre><code
              ><span class="comment"
                ># start the desktop viewer (best to do this in a separate terminal)</span
              >
<span class="prompt">$</span> otel-desktop-viewer

<span class="comment"
                ># configure otel-cli to send to our desktop viewer endpoint</span
              >
<span class="prompt">$</span
              > export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318

<span class="comment"># use otel-cli to generate spans!</span>
<span class="prompt">$</span
              > otel-cli exec --service my-service --name "curl google" curl https://google.com</code
            ></pre>
        </div>
      </div>
    </div>

    <!-- Footer -->
    <div class="card-footer justify-center pt-4">
      <p class="text-sm text-base-content/60 text-center">
        Made with
        <img
          src="/src/assets/images/axolotl.svg"
          alt="axolotl emoji"
          class="inline w-5 h-5 mx-1"
        />
        by
        <a
          href="https://github.com/CtrlSpice"
          class="link link-primary"
          target="_blank"
          rel="noopener noreferrer"
        >
          Mila Ardath
        </a>
        , with Artwork by
        <a
          href="https://cbatesonart.artstation.com/"
          class="link link-primary"
          target="_blank"
          rel="noopener noreferrer"
        >
          Chelsey Bateson
        </a>
      </p>
    </div>
  </div>
</div>
