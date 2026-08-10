package desktopexporter

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// The queue defaults are load-bearing, not incidental: NumConsumers must stay 1
// while the store has a single appender connection, and WaitForResult false is
// the async behaviour the queue exists for. Pin them so a change is a decision.
func TestDefaultSendingQueue(t *testing.T) {
	cfg := createDefaultConfig().(*Config)

	require.True(t, cfg.SendingQueue.HasValue(), "queue should be enabled by default")
	q := cfg.SendingQueue.Get()

	assert.Equal(t, 1, q.NumConsumers)
	assert.False(t, q.WaitForResult)
	assert.True(t, q.BlockOnOverflow)
	assert.Equal(t, exporterhelper.RequestSizerTypeItems, q.Sizer)
	assert.Positive(t, q.QueueSize)
	require.True(t, q.Batch.HasValue(), "batching at queue consumption should be on by default")
	assert.Positive(t, q.Batch.Get().FlushTimeout)

	require.NoError(t, cfg.Validate())
}

// Users must be able to opt back into the synchronous write path, where the
// client blocks on the store write and sees its error.
func TestSendingQueueDisable(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	conf := confmap.NewFromStringMap(map[string]any{
		"sending_queue": map[string]any{"enabled": false},
	})

	require.NoError(t, conf.Unmarshal(cfg))
	assert.False(t, cfg.SendingQueue.HasValue())
	require.NoError(t, cfg.Validate())
}

// A nonsense queue config must fail Config.Validate: recursive collector
// validation does not reach inside the Optional wrapper, so this is our check
// or nobody's.
func TestSendingQueueValidation(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	conf := confmap.NewFromStringMap(map[string]any{
		"sending_queue": map[string]any{"num_consumers": 0},
	})

	require.NoError(t, conf.Unmarshal(cfg))
	err := cfg.Validate()
	require.ErrorContains(t, err, "invalid sending_queue")
	require.ErrorContains(t, err, "num_consumers")
}

// End to end through the async path: ConsumeTraces returns after enqueue, and
// the span must still come out of a JSON-RPC searchTraces afterwards. This is
// the test that fails if the queue swallows batches, if batching mangles the
// pdata, or if shutdown drops what was still queued.
func TestQueuedIngestEndToEnd(t *testing.T) {
	ctx := context.Background()
	set := testExporterSettings(t)
	host, endpoint := startTestExtension(t)
	cfg := createDefaultConfig().(*Config)

	exp, err := createTracesExporter(ctx, set, cfg)
	require.NoError(t, err)
	require.NoError(t, exp.Start(ctx, host))
	defer func() { require.NoError(t, exp.Shutdown(ctx)) }()

	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "queue-e2e")
	span := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.SetTraceID(pcommon.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
	span.SetSpanID(pcommon.SpanID{1, 2, 3, 4, 5, 6, 7, 8})
	span.SetName("queued-span")
	now := time.Now()
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(now))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(now.Add(time.Millisecond)))

	require.NoError(t, exp.ConsumeTraces(ctx, td))

	// WaitForResult is false, so ConsumeTraces proved only that the batch was
	// enqueued; the write happens on the consumer goroutine after the batcher's
	// flush timeout. Poll the real RPC surface until it lands.
	searchBody := `{"jsonrpc":"2.0","id":1,"method":"searchTraces","params":["0","9223372036854775807"]}`
	assert.Eventually(t, func() bool {
		resp, err := http.Post("http://"+endpoint+"/rpc", "application/json",
			bytes.NewBufferString(searchBody))
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		return err == nil && strings.Contains(string(body), "queue-e2e")
	}, 5*time.Second, 50*time.Millisecond,
		"span consumed through the queue never became visible over RPC")
}
