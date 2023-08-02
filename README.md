# otel-desktop-viewer

`otel-desktop-viewer` is a CLI tool for receiving OpenTelemetry traces while working
on your local machine that helps you visualize and explore your trace data without
needing to send it on to a telemetry vendor.

![otel-desktop-viewer demo 3 LQ](https://user-images.githubusercontent.com/56372758/218345612-381fe2ff-8245-429f-ba2f-ca6431585a16.gif)

Its goals are to be easy-to-install with minimal dependencies and fast. It is written in Go
as a custom exporter on top of the [OpenTelemetry Collector](https://github.com/open-telemetry/opentelemetry-collector).
Also, it has a dark mode.

![OpenTelemetryDesktopViewer](https://user-images.githubusercontent.com/56372758/217080670-3001cb67-ab20-4ae2-ac55-82ca04bad815.png)

## Getting started

#### via Homebrew
```bash
brew tap CtrlSpice/homebrew-otel-desktop-viewer
brew install otel-desktop-viewer
```

#### via `go install`
Make sure you have [go](https://go.dev/) installed.

```bash
# install the CLI tool
go install github.com/CtrlSpice/otel-desktop-viewer@latest

# run it!
$(go env GOPATH)/bin/otel-desktop-viewer

# if you have $GOPATH/bin added to your $PATH you can call it directly!
otel-desktop-viewer

# if not you can add it to your $PATH by running this or adding it to
# your startup script (usually ~/.bashrc or ~/.zshrc)
export PATH="$(go env GOPATH)/bin:$PATH"
```

Running the CLI will open a browser tab to `localhost:8000` to load the UI,
and spin up a server listening on `localhost:4318` for OTLP http payloads and
`localhost:4317` for OTLP grpc payloads.

## Command Line Options
```bash
Flags:
      --browser int   The port number where we expose our data (default 8000)
      --grpc int      The port number on which we listen for OTLP grpc payloads (default 4317)
  -h, --help          help for otel-desktop-viewer
      --http int      The port number on which we listen for OTLP http payloads (default 4318)
  -v, --version       version for otel-desktop-viewer
```

## Configuring your OpenTelemetry SDK

To send telemetry to `otel-desktop-viewer` from your application, you need to
configure an OTLP exporter to send via grpc to `http://localhost:4317` or via
http to `http://localhost:4318`.

If your OpenTelemetry SDK OTLP exporter supports [configuration via environment
variables](https://opentelemetry.io/docs/concepts/sdk-configuration/otlp-exporter-configuration/)
then you should be able to send to `otel-desktop-viewer` with the following environment
variables set

```
# For HTTP:
export OTEL_EXPORTER_OTLP_ENDPOINT="http://localhost:4318"
export OTEL_TRACES_EXPORTER="otlp"
export OTEL_EXPORTER_OTLP_PROTOCOL="http/protobuf"

# For GRPC:
export OTEL_EXPORTER_OTLP_ENDPOINT="http://localhost:4317"
export OTEL_TRACES_EXPORTER="otlp"
export OTEL_EXPORTER_OTLP_PROTOCOL="grpc"
```
## Keyboard navigation and shortcuts
```bash
Navigation:
    Move up the trace summary list:      ← or h 
    Move down the trace summary list:    → or l 
    Move up the span list:               ↑ or k
    Move down the span list:             ↓ or j

Shortcuts:
    Clear all traces:                    ctrl + l 
    Refresh the page:                    r
    Bring up the keyboard help dialog:   ? 
```


## Example with `otel-cli`

If you have [`otel-cli`](https://github.com/equinix-labs/otel-cli) installed, you can
send some example data with the following script.

```bash
# start the desktop viewer (best to do this in a separate terminal)
otel-desktop-viewer

# configure otel-cli to send to our desktop viewer endpoint
export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318

# use otel-cli to generate spans!
otel-cli exec --service my-service --name "curl google" curl https://google.com

# a more visually interesting example trace
carrier=$(mktemp)
sockdir=$(mktemp -d)
otel-cli span background \
   --service "otel-cli-example" \
   --name "otel-cli-example background span" \
   --tp-print \
   --tp-carrier $carrier \
   --sockdir $sockdir &
sleep 0.1 # give the background server just a few ms to start up
otel-cli span event --name "cool thing" --attrs "foo=bar" --sockdir $sockdir
otel-cli exec --service "otel-cli-example" --name "curl google" --tp-carrier $carrier curl https://google.com
sleep 0.1
otel-cli exec --service "otel-cli-example" --name "curl google" --tp-carrier $carrier curl https://google.com
sleep 0.1
otel-cli span event --name "another cool thing\!" --attrs "foo=bar" --sockdir $sockdir
otel-cli span end --sockdir $sockdir
```

![otel-cli-example](https://user-images.githubusercontent.com/56372758/217082956-23c60f2d-f882-4c78-a205-f744596fac21.png)

## Why does this exist?

When we send OpenTelemetry telemetry to a tracing vendor we expect to be able to visualize our
data, but when working locally our experience often looks more like this:

```
{
	"Name": "Poll",
	"SpanContext": {
		"TraceID": "4d7f165e90111f4fc9003d5bbf7aca81",
		"SpanID": "1f7a655a9b7a6f4b",
		"TraceFlags": "01",
		"TraceState": "",
		"Remote": false
	},
	"Parent": {
		"TraceID": "4d7f165e90111f4fc9003d5bbf7aca81",
		"SpanID": "d1b8f7f96ad9d1a0",
		"TraceFlags": "01",
		"TraceState": "",
		"Remote": false
	},
  ...
```

You can use [Jaeger's all-in-one](https://www.jaegertracing.io/docs/1.41/deployment/#all-in-one)
distribution, but this requires quite a bit of additional knowledge for the end-user around docker and
navigating a lot of configuration options. Additionally the user experience is not focused around 
"show me the data I just emitted".

The goals for `otel-desktop-viewer` are to allow a user to install it with one command, require
minimal configuration and additional tooling, and be as approachable as possible for developers
at all levels of experience.


## Implementation

The CLI is implemented in Go building on top of the OpenTelemetry Collector. A custom
`desktop` exporter is registered that:

- collects trace data in memory
- exposes this trace data via an HTTP API
- serves a static React app that renders the collected traces

All of the static web assets are built into the final binary using the [go:embed](https://blog.jetbrains.com/go/2021/06/09/how-to-use-go-embed-in-go-1-16/)
directive so that the binary is self-contained and relocatable.

## What's with the axolotl??

Her name is Lulu Axol'Otel, she is very pink, and I love her.

More seriously, I like to give my [side projects](https://github.com/CtrlSpice/bumblebee-consolematch) an 
[animal theme](https://github.com/CtrlSpice/yak-vs-yak) to add a little aesthetic
interest on what otherwise might be fairly plain applications.

## License
Apache 2.0, see LICENSE
