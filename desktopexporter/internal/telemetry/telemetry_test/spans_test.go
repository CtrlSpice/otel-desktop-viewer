package telemetry_test

import (
	"log"
	"testing"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry"
	"github.com/stretchr/testify/assert"
)

// Validate resource data
type attributeTest struct {
	key           string
	expectedValue any
}

var resourceAttributes = []attributeTest{
	{"service.name", "sample.currencyservice"},
	{"telemetry.sdk.language", "cpp"},
	{"telemetry.sdk.name", "opentelemetry"},
	{"telemetry.sdk.version", "1.5.0"},
	{"array.example", []any{"example1", "example2", "example3"}},
}

var scopeAttributes = []attributeTest{
	{"owner.name", "Mila Ardath"},
	{"owner.contact", "github.com/CtrlSpice"},
}

var eventAttributes = []attributeTest{
	{"event.class", "sample"},
	{"event.priority", int64(1)},
}

var spanAttributes = []attributeTest{
	{"http.flavor", "1.1"},
	{"http.host", "frontend:8080"},
	{"http.method", "POST"},
	{"http.request_length", int64(102)},
	{"http.status_code", int64(200)},
	{"http.status_text", "Ok"},
	{"http.target", "/api/cart"},
	{"http.url", "http://frontend:8080/api/cart"},
	{"http.user_agent", "python-requests/2.27.1"},
	{"net.host.ip", "::ffff:172.24.0.22"},
	{"net.host.name", "frontend"},
	{"net.host.port", int64(8080)},
	{"net.peer.ip", "::ffff:172.24.0.23"},
	{"net.peer.port", int64(46054)},
	{"net.transport", "ip_tcp"},
	{"array.example", []any{1.1, 1.2, 1.3}},
}

var spans []telemetry.SpanData

func init() {
	spans = telemetry.NewSampleTelemetry().Spans
}

func TestExtractSpans(t *testing.T) {
	// Validate number of spans extraced from the sample telemetry
	assert.Len(t, spans, 4)
}

func TestSpanResource(t *testing.T) {
	resource := spans[0].Resource

	// Dropped resource attribute count
	assert.Equal(t, uint32(0), resource.DroppedAttributesCount)

	// Resource attributes
	for _, attr := range resourceAttributes {
		if attr.key == "array.example" {
			log.Println(resource.Attributes[attr.key])
		}
		assert.Equal(t, attr.expectedValue, resource.Attributes[attr.key])
	}
}

func TestSpanScope(t *testing.T) {
	scope := spans[0].Scope

	// Scope name
	assert.Equal(t, "sample.currencyservice", scope.Name)

	// Scope version
	assert.Equal(t, "v1.2.3", scope.Version)

	// Dropped scope attributes count
	assert.Equal(t, uint32(2), scope.DroppedAttributesCount)

	// Scope attributes
	assert.Equal(t, uint32(2), scope.DroppedAttributesCount)
	for _, attr := range scopeAttributes {
		assert.Equal(t, attr.expectedValue, scope.Attributes[attr.key])
	}
}

func TestSpanEvents(t *testing.T) {
	event := spans[0].Events[1]

	// Event name
	assert.Equal(t, "Conversion successful. Response sent back.", event.Name)

	// Event timestamp
	assert.Equal(t, time.Date(2023, 02, 01, 20, 25, 36, 179479924, time.UTC), event.Timestamp)

	// Dropped event attributes count
	assert.Equal(t, uint32(1), event.DroppedAttributesCount)

	// Event attributes
	for _, attr := range eventAttributes {
		assert.Equal(t, attr.expectedValue, event.Attributes[attr.key])
	}
}

func TestSpanLinks(t *testing.T) {
	link := spans[2].Links[0]

	// Link span ID
	assert.Equal(t, "2c1ae93af4d3f887", link.SpanID)

	// Link trace ID
	assert.Equal(t, "7979cec4d1c04222fa9a3c7c97c0a99c", link.TraceID)

	// Dropped link attributes count
	assert.Equal(t, uint32(5), link.DroppedAttributesCount)

	// Link attributes
	assert.Equal(t, "in-cart currency conversion", link.Attributes["relationship"])
}

func TestSpan(t *testing.T) {
	span := spans[3]

	// Dropped attributes count
	assert.Equal(t, uint32(0), span.DroppedAttributesCount)

	// Dropped events count
	assert.Equal(t, uint32(0), span.DroppedEventsCount)

	// Dropped links count
	assert.Equal(t, uint32(0), span.DroppedLinksCount)

	// Span name
	assert.Equal(t, "SAMPLE HTTP POST", span.Name)

	// Span kind
	assert.Equal(t, "Server", span.Kind)

	// Start time
	assert.Equal(t, time.Date(2023, 02, 02, 18, 17, 54, 805039872, time.UTC), span.StartTime)

	// End time
	assert.Equal(t, time.Date(2023, 02, 02, 18, 17, 54, 816274688, time.UTC), span.EndTime)

	// Span kind
	assert.Equal(t, "Unset", span.StatusCode)

	// Span ID
	assert.Equal(t, "355dc9bea1ec64d8", span.SpanID)

	// Parent span ID
	assert.Equal(t, "a24ac1588d52a6fc", span.ParentSpanID)

	// Trace ID
	assert.Equal(t, "42957c7c2fca940a0d32a0cdd38c06a4", span.TraceID)

	// Span attributes
	for _, attr := range spanAttributes {
		assert.Equal(t, attr.expectedValue, span.Attributes[attr.key])
	}
}
