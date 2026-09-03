//go:build waterfallbench

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/server"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"golang.org/x/exp/jsonrpc2"
)

func TestRunWithoutArgumentsPrintsPositiveControl(t *testing.T) {
	var stdout bytes.Buffer
	require.NoError(t, run(context.Background(), nil, &stdout))
	require.Equal(t, benchmarkSentinel+"\n", stdout.String())
}

func TestRunHelpPrintsUsageWithoutError(t *testing.T) {
	var stdout bytes.Buffer
	require.NoError(t, run(context.Background(), []string{"serve", "--help"}, &stdout))
	require.Equal(t, serveUsage+"\n", stdout.String())
}

func TestParseCommand(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want commandOptions
	}{
		{name: "positive control", want: commandOptions{}},
		{name: "serve defaults", args: []string{"serve"}, want: commandOptions{serve: true, listen: defaultListen, armCListen: defaultArmCListen}},
		{name: "custom listen", args: []string{"serve", "--listen", "127.0.0.1:0"}, want: commandOptions{serve: true, listen: "127.0.0.1:0", armCListen: defaultArmCListen}},
		{name: "custom benchmark listen", args: []string{"serve", "--benchmark-listen", "127.0.0.1:0"}, want: commandOptions{serve: true, listen: defaultListen, armCListen: "127.0.0.1:0"}},
		{name: "equals syntax", args: []string{"serve", "--listen=localhost:9000", "--benchmark-listen=localhost:9001"}, want: commandOptions{serve: true, listen: "localhost:9000", armCListen: "localhost:9001"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseCommand(test.args)
			require.NoError(t, err)
			require.Equal(t, test.want, got)
		})
	}
}

func TestParseCommandRejectsInvalidArguments(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		message string
	}{
		{name: "unknown command", args: []string{"benchmark"}, message: "unknown command"},
		{name: "extra argument", args: []string{"serve", "extra"}, message: "unexpected arguments"},
		{name: "unknown flag", args: []string{"serve", "--port", "8001"}, message: "flag provided but not defined"},
		{name: "missing listen value", args: []string{"serve", "--listen"}, message: "flag needs an argument"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseCommand(test.args)
			require.ErrorContains(t, err, test.message)
		})
	}
}

func TestCloseWithContextIsBounded(t *testing.T) {
	release := make(chan struct{})
	finished := make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := closeWithContext(ctx, func() error {
		defer close(finished)
		<-release
		return nil
	})
	require.ErrorIs(t, err, context.DeadlineExceeded)

	close(release)
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("timed-out close operation did not finish after release")
	}
}

func TestCloseWithContextReturnsCloseError(t *testing.T) {
	want := errors.New("close failed")
	err := closeWithContext(context.Background(), func() error { return want })
	require.ErrorIs(t, err, want)
}

type productionTraceResponse struct {
	TraceID           string                     `json:"traceID"`
	Resources         map[string]json.RawMessage `json:"resources"`
	Scopes            map[string]json.RawMessage `json:"scopes"`
	UnplacedSpanCount int                        `json:"unplacedSpanCount"`
	Spans             []productionSpanNode       `json:"spans"`
}

type productionSpanNode struct {
	SpanData   productionSpanData `json:"spanData"`
	Depth      int                `json:"depth"`
	Matched    bool               `json:"matched"`
	Salvaged   bool               `json:"salvaged"`
	CyclePoint bool               `json:"cyclePoint"`
}

type productionSpanData struct {
	SpanID       string  `json:"spanID"`
	ParentSpanID *string `json:"parentSpanID"`
	Start        uint64  `json:"start"`
	Resource     int64   `json:"r"`
	Scope        int64   `json:"s"`
}

func TestCheckedFixturesThroughProductionSearchSpans(t *testing.T) {
	ctx := context.Background()
	benchmarkStore, err := store.NewStore(ctx, "", zap.NewNop())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, benchmarkStore.Close()) })

	rejected, err := ingestFixtures(ctx, benchmarkStore)
	require.NoError(t, err)
	require.Zero(t, rejected, "checked fixtures must ingest without rejected spans")

	manifest, err := loadEmbeddedFixtureManifest()
	require.NoError(t, err)
	require.Len(t, manifest.Fixtures, 7)
	handler := server.NewJSONRPCHandler(benchmarkStore, zap.NewNop())

	for _, fixture := range manifest.Fixtures {
		t.Run(fixture.Name, func(t *testing.T) {
			params, err := json.Marshal(map[string]any{"traceID": fixture.TraceID, "query": nil})
			require.NoError(t, err)
			result, err := handler.Handle(ctx, &jsonrpc2.Request{
				Method: "searchSpans",
				Params: params,
				ID:     jsonrpc2.Int64ID(1),
			})
			require.NoError(t, err)
			raw, ok := result.(json.RawMessage)
			require.Truef(t, ok, "searchSpans returned %T, not json.RawMessage", result)

			var trace productionTraceResponse
			require.NoError(t, json.Unmarshal(raw, &trace))
			require.Equal(t, fixture.TraceID, trace.TraceID)
			require.Len(t, trace.Spans, fixture.ExpectedDisplayedSpanCount)
			require.Equal(t, fixture.ExpectedFirstSpanID, trace.Spans[0].SpanData.SpanID)
			require.Zero(t, trace.UnplacedSpanCount)
			require.NotEmpty(t, trace.Resources)
			require.NotEmpty(t, trace.Scopes)

			maximumDepth := -1
			ids := make([]string, len(trace.Spans))
			depths := make([]int, len(trace.Spans))
			for i, span := range trace.Spans {
				ids[i] = span.SpanData.SpanID
				depths[i] = span.Depth
				maximumDepth = max(maximumDepth, span.Depth)
				require.Truef(t, span.Matched, "span %s was not matched", span.SpanData.SpanID)

				resourceKey := strconv.FormatInt(span.SpanData.Resource, 10)
				_, resourceFound := trace.Resources[resourceKey]
				require.Truef(t, resourceFound, "span %s references absent resource %s", span.SpanData.SpanID, resourceKey)
				scopeKey := strconv.FormatInt(span.SpanData.Scope, 10)
				_, scopeFound := trace.Scopes[scopeKey]
				require.Truef(t, scopeFound, "span %s references absent scope %s", span.SpanData.SpanID, scopeKey)

				if fixture.Name != "cycle" {
					require.False(t, span.Salvaged)
					require.False(t, span.CyclePoint)
				}
			}
			require.Equal(t, fixture.ExpectedMaximumDisplayedDepth, maximumDepth)

			switch fixture.Name {
			case "multiple-roots":
				require.Equal(t, []string{
					"0000000000000001", "0000000000000002",
					"0000000000000003", "0000000000000004",
					"0000000000000005", "0000000000000006",
				}, ids)
				require.Equal(t, []int{0, 1, 0, 1, 0, 1}, depths)
			case "orphan":
				require.Equal(t, []string{
					"0000000000000001", "0000000000000002",
					"0000000000000003", "0000000000000004",
				}, ids)
				require.Equal(t, []int{0, 1, 0, 1}, depths)
				require.Nil(t, trace.Spans[0].SpanData.ParentSpanID)
				require.NotNil(t, trace.Spans[2].SpanData.ParentSpanID)
				require.Equal(t, "00000000000003e7", *trace.Spans[2].SpanData.ParentSpanID)
				require.Greater(t, trace.Spans[0].SpanData.Start, trace.Spans[2].SpanData.Start,
					"the genuine root must display before the earlier orphan")
			case "cycle":
				require.Equal(t, []string{
					"0000000000000001", "0000000000000002", "0000000000000003",
				}, ids)
				require.Equal(t, []int{0, 1, 2}, depths)
				require.Equal(t, []bool{true, true, true}, []bool{
					trace.Spans[0].Salvaged, trace.Spans[1].Salvaged, trace.Spans[2].Salvaged,
				})
				require.Equal(t, []bool{true, false, false}, []bool{
					trace.Spans[0].CyclePoint, trace.Spans[1].CyclePoint, trace.Spans[2].CyclePoint,
				}, "exactly the salvaged display root must be the cycle point")
				require.Zero(t, trace.UnplacedSpanCount)
			}
		})
	}
}
