//go:build evals

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/extension/extensiontest"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/receiver/receivertest"
)

func startEvalCollectorIntegration(t *testing.T, db, flags string) (string, func()) {
	t.Helper()
	o := testOptions()
	o.host = "127.0.0.1"
	o.browserPort, o.httpPort, o.grpcPort = redactionTestPort(t), redactionTestPort(t), redactionTestPort(t)
	o.db, o.dbMaxSize, o.featureFlagsFile = db, "0", flags
	cfg, err := resolveConfig(t, o)
	require.NoError(t, err)
	require.NoError(t, cfg.Validate())
	factories, err := components()
	require.NoError(t, err)
	extID := component.MustNewID("duckdb")
	ext, err := factories.Extensions[extID.Type()].Create(t.Context(), extensiontest.NewNopSettings(extID.Type()), cfg.Extensions[extID])
	require.NoError(t, err)
	require.NoError(t, ext.Start(t.Context(), componenttest.NewNopHost()))
	started := []component.Component{ext}
	var once sync.Once
	stop := func() {
		once.Do(func() {
			for i := len(started) - 1; i >= 0; i-- {
				require.NoError(t, started[i].Shutdown(context.Background()))
			}
		})
	}
	t.Cleanup(stop)
	expID := component.MustNewID("desktop")
	exp, err := factories.Exporters[expID.Type()].CreateLogs(t.Context(), exporter.Settings{ID: expID, TelemetrySettings: componenttest.NewNopTelemetrySettings()}, cfg.Exporters[expID])
	require.NoError(t, err)
	require.NoError(t, exp.Start(t.Context(), attributeIntegrationHost{ext: ext}))
	started = append(started, exp)
	batchID := component.MustNewID("batch")
	batch, err := factories.Processors[batchID.Type()].CreateLogs(t.Context(), processor.Settings{ID: batchID, TelemetrySettings: componenttest.NewNopTelemetrySettings()}, cfg.Processors[batchID], exp)
	require.NoError(t, err)
	require.NoError(t, batch.Start(t.Context(), componenttest.NewNopHost()))
	started = append(started, batch)
	redactID := component.MustNewID("redaction")
	redact, err := factories.Processors[redactID.Type()].CreateLogs(t.Context(), processor.Settings{ID: redactID, TelemetrySettings: componenttest.NewNopTelemetrySettings()}, cfg.Processors[redactID], batch)
	require.NoError(t, err)
	require.NoError(t, redact.Start(t.Context(), componenttest.NewNopHost()))
	started = append(started, redact)
	recID := component.MustNewID("otlp")
	receiver, err := factories.Receivers[recID.Type()].CreateLogs(t.Context(), receivertest.NewNopSettings(recID.Type()), cfg.Receivers[recID], redact)
	require.NoError(t, err)
	require.NoError(t, receiver.Start(t.Context(), componenttest.NewNopHost()))
	started = append(started, receiver)
	return "http://" + formatEndpoint(o.host)(o.browserPort), stop
}

func TestEvalCollectorBatchQueuePersistenceAndReopen(t *testing.T) {
	dir := t.TempDir()
	flags := filepath.Join(dir, "flags.json")
	require.NoError(t, os.WriteFile(flags, []byte(`{"flags":{"evals.enabled":{"state":"ENABLED","variants":{"on":true},"defaultVariant":"on"}}}`), 0600))
	db := filepath.Join(dir, "evals.duckdb")
	viewer, stop := startEvalCollectorIntegration(t, db, flags)
	input, err := os.ReadFile("desktopexporter/internal/evaldecode/testdata/promptfoo-v3.json")
	require.NoError(t, err)
	post := func(endpoint string) []byte {
		response, err := http.Post(endpoint+"/api/evals/ingest?name=synthetic.json", "application/json", bytes.NewReader(input))
		require.NoError(t, err)
		body, err := io.ReadAll(response.Body)
		require.NoError(t, response.Body.Close())
		require.NoError(t, err)
		require.Equal(t, 200, response.StatusCode, string(body))
		return body
	}
	first := post(viewer)
	require.Contains(t, string(first), `"complete":true`)
	_, result, err := requestQuery(t.Context(), http.DefaultClient, viewer, `select (select count(*) from logs)::varchar, (select count(*) from eval_records)::varchar, (select count(*) from spans)::varchar, (select count(*) from logs where contains(body::varchar,'synthetic-secret'))::varchar`, 1)
	require.NoError(t, err)
	require.Equal(t, []any{"3", "1", "0", "0"}, result.Rows[0])
	stop()
	viewer, _ = startEvalCollectorIntegration(t, db, flags)
	second := post(viewer)
	require.Contains(t, string(second), `"created":false`)
	var a, b struct {
		IDs []string `json:"ids"`
	}
	require.NoError(t, json.Unmarshal(first, &a))
	require.NoError(t, json.Unmarshal(second, &b))
	require.Equal(t, a.IDs, b.IDs)
	_, result, err = requestQuery(t.Context(), http.DefaultClient, viewer, `select count(*)::varchar from logs`, 1)
	require.NoError(t, err)
	require.Equal(t, "3", result.Rows[0][0])
}
