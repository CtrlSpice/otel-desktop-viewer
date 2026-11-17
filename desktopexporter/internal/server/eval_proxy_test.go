//go:build evals

package server

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/evaldecode"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/plog/plogotlp"
	"go.uber.org/zap"
)

func TestEvalForwardingBypassesEnvironmentProxy(t *testing.T) {
	// ProxyFromEnvironment caches process environment on its first invocation.
	if os.Getenv("VIEWER_EVAL_PROXY_TEST_CHILD") != "1" {
		executable, err := os.Executable()
		require.NoError(t, err)
		cmd := exec.CommandContext(t.Context(), executable, "-test.run=^TestEvalForwardingBypassesEnvironmentProxy$")
		cmd.Env = append(os.Environ(), "VIEWER_EVAL_PROXY_TEST_CHILD=1")
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, string(output))
		return
	}
	var proxyCalls, dials atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyCalls.Add(1)
		http.Error(w, "unexpected pre-redaction proxy traffic", 502)
	}))
	defer proxy.Close()
	for _, key := range []string{"HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy"} {
		t.Setenv(key, proxy.URL)
	}
	for _, key := range []string{"NO_PROXY", "no_proxy", "REQUEST_METHOD"} {
		t.Setenv(key, "")
	}
	const authority = "192.168.1.20:4318"
	request, err := http.NewRequest(http.MethodPost, "http://"+authority+"/v1/logs", nil)
	require.NoError(t, err)
	selected, err := http.ProxyFromEnvironment(request)
	require.NoError(t, err)
	require.NotNil(t, selected)
	require.Equal(t, proxy.URL, selected.String())
	type received struct {
		path, host, contentType string
		body                    []byte
		err                     error
	}
	arrivals := make(chan received, 2)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		arrivals <- received{r.URL.Path, r.Host, r.Header.Get("Content-Type"), body, err}
		w.Header().Set("Content-Type", "application/x-protobuf")
	}))
	defer receiver.Close()
	s, err := NewServer("192.168.1.20:0", 4318, nil, zap.NewNop(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Shutdown(context.Background())) })
	transport := s.evalReceiverClient.Transport.(*http.Transport)
	// Map only the intended receiver to our local listener. A proxy dial remains
	// a real proxy dial, so accidental proxy selection cannot pass unnoticed.
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if address == authority {
			dials.Add(1)
			address = receiver.Listener.Addr().String()
		}
		return (&net.Dialer{}).DialContext(ctx, network, address)
	}
	for range 2 {
		require.NoError(t, s.forwardEvalPart(t.Context(), strings.Repeat("a", 64), evaldecode.IngestPart{Payload: json.RawMessage(`{"password":"synthetic-before-redaction"}`)}))
		got := <-arrivals
		require.NoError(t, got.err)
		require.Equal(t, "/v1/logs", got.path)
		require.Equal(t, authority, got.host)
		require.Equal(t, "application/x-protobuf", got.contentType)
		exported := plogotlp.NewExportRequest()
		require.NoError(t, exported.UnmarshalProto(got.body))
		source, ok := exported.Logs().ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0).Body().Map().Get("source")
		require.True(t, ok)
		password, ok := source.Map().Get("password")
		require.True(t, ok)
		require.Equal(t, "synthetic-before-redaction", password.Str())
	}
	require.Zero(t, proxyCalls.Load())
	require.Equal(t, int32(1), dials.Load(), "reuse the internal connection pool across parts")
}
