package store

import (
	"testing"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry"
	"github.com/stretchr/testify/assert"
)

// createTestLogs creates a comprehensive set of test logs
func createTestLogs(baseTime int64) []telemetry.LogData {
	return []telemetry.LogData{
		{
			Timestamp:         baseTime,
			ObservedTimestamp: baseTime + 100 * time.Millisecond.Nanoseconds(),
			TraceID:          "test-trace",
			SpanID:           "root-span",
			SeverityText:     "INFO",
			SeverityNumber:   9,
			Body: map[string]any{
				"message": "Root operation started",
				"details": map[string]any{
					"operation": "root",
					"status": "starting",
					"metrics": map[string]any{
						"cpu": 42.5,
						"mem": 1024,
					},
				},
			},
			Resource: &telemetry.ResourceData{
				Attributes: map[string]any{
					"service.name":    "test-service",
					"service.version": "1.0.0",
				},
				DroppedAttributesCount: 0,
			},
			Scope: &telemetry.ScopeData{
				Name:                   "test-scope",
				Version:                "v1.0.0",
				Attributes:             map[string]any{},
				DroppedAttributesCount: 0,
			},
			Attributes: map[string]any{
				"log.string": "root-log",
				"log.int":    int64(42),
				"log.float":  float64(3.14),
				"log.bool":   true,
				"log.list":   []string{"one", "two", "three"},
			},
			DroppedAttributesCount: 0,
			Flags:                  0,
			EventName:              "root.event",
		},
		{
			// This log has zero timestamp, should fall back to observed timestamp
			Timestamp:             0, // Explicitly set to zero time
			ObservedTimestamp:     baseTime + 150 * time.Millisecond.Nanoseconds(),
			TraceID:               "test-trace",
			SpanID:                "child-span",
			SeverityText:          "ERROR",
			SeverityNumber:        17,
			Body:                  "Child operation failed",
			Resource: &telemetry.ResourceData{
				Attributes: map[string]any{
					"service.name":    "test-service",
					"service.version": "1.0.0",
				},
				DroppedAttributesCount: 1,
			},
			Scope: &telemetry.ScopeData{
				Name:                   "test-scope",
				Version:                "v1.0.0",
				Attributes:             map[string]any{},
				DroppedAttributesCount: 0,
			},
			Attributes: map[string]any{
				"log.string": "child-log",
				"log.int":    int64(24),
				"log.float":  float64(2.71),
				"log.bool":   false,
				"log.list":   []int64{1, 2, 3, 4, 5},
			},
			DroppedAttributesCount: 1,
			Flags:                  1,
			EventName:              "child.event",
		},
		{
			Timestamp:         baseTime + 100 * time.Millisecond.Nanoseconds(),
			ObservedTimestamp: baseTime + 200 * time.Millisecond.Nanoseconds(),
			TraceID:          "test-trace",
			SpanID:           "orphaned-span",
			SeverityText:     "WARN",
			SeverityNumber:   13,
			Body:            "Orphaned operation",
			Resource: &telemetry.ResourceData{
				Attributes:             map[string]any{},
				DroppedAttributesCount: 0,
			},
			Scope: &telemetry.ScopeData{
				Name:                   "test-scope",
				Version:                "v1.0.0",
				Attributes:             map[string]any{},
				DroppedAttributesCount: 0,
			},
			Attributes: map[string]any{
				"log.string": "orphaned-log",
			},
			DroppedAttributesCount: 0,
			Flags:                  0,
			EventName:              "orphaned.event",
		},
	}
}

// TestLogSuite runs a comprehensive suite of tests on a set of logs
func TestLogSuite(t *testing.T) {
	helper, teardown := setupTest(t)
	defer teardown()

	baseTime := time.Now().UnixNano()
	logs := createTestLogs(baseTime)
	err := helper.store.AddLogs(helper.ctx, logs)
	assert.NoError(t, err, "failed to add test logs")

	t.Run("LogOrdering", func(t *testing.T) {
		allLogs, err := helper.store.GetLogs(helper.ctx)
		assert.NoError(t, err)
		assert.Len(t, allLogs, 3, "should have three logs")

		// Verify logs are ordered by timestamp (newest first)
		// Note: Child log with no timestamp uses observed timestamp (t+150ms) for ordering
		assert.Equal(t, "child-span", allLogs[0].SpanID, "first log should be child (newest, t+150ms)")
		assert.Equal(t, "orphaned-span", allLogs[1].SpanID, "second log should be orphaned (middle, t+100ms)")
		assert.Equal(t, "root-span", allLogs[2].SpanID, "third log should be root (oldest, t+0ms)")
	})

	t.Run("LogSeverity", func(t *testing.T) {
		logs, err := helper.store.GetLogs(helper.ctx)
		assert.NoError(t, err)

		// Verify child log severity (middle)
		assert.Equal(t, "ERROR", logs[0].SeverityText, "child log severity text")
		assert.Equal(t, int32(17), logs[0].SeverityNumber, "child log severity number")

		// Verify orphaned log severity (newest)
		assert.Equal(t, "WARN", logs[1].SeverityText, "orphaned log severity text")
		assert.Equal(t, int32(13), logs[1].SeverityNumber, "orphaned log severity number")


		// Verify root log severity (oldest)
		assert.Equal(t, "INFO", logs[2].SeverityText, "root log severity text")
		assert.Equal(t, int32(9), logs[2].SeverityNumber, "root log severity number")
	})

	t.Run("LogBody", func(t *testing.T) {
		logs, err := helper.store.GetLogs(helper.ctx)
		assert.NoError(t, err)

		// Verify child log body
		assert.Equal(t, "Child operation failed", logs[0].Body, "child log body")

		// Verify orphaned log body
		assert.Equal(t, "Orphaned operation", logs[1].Body, "orphaned log body")

		// Verify root log body (nested map)
		rootBody := logs[2].Body.(map[string]any)
		assert.Equal(t, "Root operation started", rootBody["message"], "root log message")
		details := rootBody["details"].(map[string]any)
		assert.Equal(t, "root", details["operation"], "root log operation")
		assert.Equal(t, "starting", details["status"], "root log status")
		metrics := details["metrics"].(map[string]any)
		assert.Equal(t, float64(42.5), metrics["cpu"], "root log cpu metric")
		assert.Equal(t, float64(1024), metrics["mem"], "root log mem metric")
	})

	t.Run("LogTimestamp", func(t *testing.T) {
		logs, err := helper.store.GetLogs(helper.ctx)
		assert.NoError(t, err)

		// Verify child log timestamp (should remain zero)
		assert.Zero(t, logs[0].Timestamp, "child log should have zero timestamp")
		assert.Equal(t, baseTime + 150 * time.Millisecond.Nanoseconds(), logs[0].ObservedTimestamp, "child log should have correct observed timestamp")
		
		// Verify orphaned log timestamp
		assert.NotZero(t, logs[1].Timestamp, "orphaned log should have timestamp")
		assert.NotZero(t, logs[1].ObservedTimestamp, "orphaned log should have observed timestamp")
		
		// Verify root log timestamp
		assert.NotZero(t, logs[2].Timestamp, "root log should have timestamp")
		assert.NotZero(t, logs[2].ObservedTimestamp, "root log should have observed timestamp")
	})

	t.Run("LogResource", func(t *testing.T) {
		logs, err := helper.store.GetLogs(helper.ctx)
		assert.NoError(t, err)

		// Verify child log resource (newest)
		assert.Equal(t, "test-service", logs[0].Resource.Attributes["service.name"], "child log service name")
		assert.Equal(t, "1.0.0", logs[0].Resource.Attributes["service.version"], "child log service version")
		assert.Equal(t, uint32(1), logs[0].Resource.DroppedAttributesCount, "child log resource dropped count")

		// Verify orphaned log resource (middle)
		assert.Empty(t, logs[1].Resource.Attributes, "orphaned log should have empty resource attributes")
		assert.Equal(t, uint32(0), logs[1].Resource.DroppedAttributesCount, "orphaned log resource dropped count")

		// Verify root log resource (oldest)
		assert.Equal(t, "test-service", logs[2].Resource.Attributes["service.name"], "root log service name")
		assert.Equal(t, "1.0.0", logs[2].Resource.Attributes["service.version"], "root log service version")
		assert.Equal(t, uint32(0), logs[2].Resource.DroppedAttributesCount, "root log resource dropped count")
	})

	t.Run("LogScope", func(t *testing.T) {
		logs, err := helper.store.GetLogs(helper.ctx)
		assert.NoError(t, err)

		// Verify scope is consistent across all logs
		for i, log := range logs {
			assert.Equal(t, "test-scope", log.Scope.Name, "log %d scope name", i)
			assert.Equal(t, "v1.0.0", log.Scope.Version, "log %d scope version", i)
			assert.Empty(t, log.Scope.Attributes, "log %d scope attributes", i)
			assert.Equal(t, uint32(0), log.Scope.DroppedAttributesCount, "log %d scope dropped count", i)
		}
	})

	t.Run("LogAttributes", func(t *testing.T) {
		logs, err := helper.store.GetLogs(helper.ctx)
		assert.NoError(t, err)

		// Verify child log attributes (newest)
		assert.Equal(t, "child-log", logs[0].Attributes["log.string"], "child log string attribute")
		assert.Equal(t, int64(24), logs[0].Attributes["log.int"], "child log int attribute")
		assert.Equal(t, float64(2.71), logs[0].Attributes["log.float"], "child log float attribute")
		assert.Equal(t, false, logs[0].Attributes["log.bool"], "child log bool attribute")
		childList := logs[0].Attributes["log.list"].([]any)
		assert.Equal(t, []any{int64(1), int64(2), int64(3), int64(4), int64(5)}, childList, "child log list attribute")
		
		// Verify orphaned log attributes (middle)
		assert.Equal(t, "orphaned-log", logs[1].Attributes["log.string"], "orphaned log string attribute")

		// Verify root log attributes (oldest)
		assert.Equal(t, "root-log", logs[2].Attributes["log.string"], "root log string attribute")
		assert.Equal(t, int64(42), logs[2].Attributes["log.int"], "root log int attribute")
		assert.Equal(t, float64(3.14), logs[2].Attributes["log.float"], "root log float attribute")
		assert.Equal(t, true, logs[2].Attributes["log.bool"], "root log bool attribute")
		rootList := logs[2].Attributes["log.list"].([]any)
		assert.Equal(t, []any{"one", "two", "three"}, rootList, "root log list attribute")
	})

	t.Run("LogMetadata", func(t *testing.T) {
		logs, err := helper.store.GetLogs(helper.ctx)
		assert.NoError(t, err)

		// Verify child log metadata
		assert.Equal(t, uint32(1), logs[0].DroppedAttributesCount, "child log dropped count")
		assert.Equal(t, uint32(1), logs[0].Flags, "child log flags")
		assert.Equal(t, "child.event", logs[0].EventName, "child log event name")
		
		// Verify orphaned log metadata
		assert.Equal(t, uint32(0), logs[1].DroppedAttributesCount, "orphaned log dropped count")
		assert.Equal(t, uint32(0), logs[1].Flags, "orphaned log flags")
		assert.Equal(t, "orphaned.event", logs[1].EventName, "orphaned log event name")

		// Verify root log metadata
		assert.Equal(t, uint32(0), logs[2].DroppedAttributesCount, "root log dropped count")
		assert.Equal(t, uint32(0), logs[2].Flags, "root log flags")
		assert.Equal(t, "root.event", logs[2].EventName, "root log event name")
	})

	t.Run("LogsByTraceSpan", func(t *testing.T) {
		// Test getting logs for root span
		rootLogs, err := helper.store.GetLogsByTraceSpan(helper.ctx, "test-trace", "root-span")
		assert.NoError(t, err)
		assert.Len(t, rootLogs, 1, "should have one root span log")
		assert.Equal(t, "root-span", rootLogs[0].SpanID, "root span log ID")

		// Test getting logs for child span
		childLogs, err := helper.store.GetLogsByTraceSpan(helper.ctx, "test-trace", "child-span")
		assert.NoError(t, err)
		assert.Len(t, childLogs, 1, "should have one child span log")
		assert.Equal(t, "child-span", childLogs[0].SpanID, "child span log ID")

		// Test getting logs for orphaned span
		orphanedLogs, err := helper.store.GetLogsByTraceSpan(helper.ctx, "test-trace", "orphaned-span")
		assert.NoError(t, err)
		assert.Len(t, orphanedLogs, 1, "should have one orphaned span log")
		assert.Equal(t, "orphaned-span", orphanedLogs[0].SpanID, "orphaned span log ID")

		// Test getting logs for non-existent trace/span
		nonExistentLogs, err := helper.store.GetLogsByTraceSpan(helper.ctx, "non-existent-trace", "non-existent-span")
		assert.NoError(t, err)
		assert.Empty(t, nonExistentLogs, "should have no logs for non-existent trace/span")
	})
}

// TestLogNotFound verifies error handling for non-existent log IDs
func TestLogNotFound(t *testing.T) {
	helper, teardown := setupTest(t)
	defer teardown()

	_, err := helper.store.GetLog(helper.ctx, "non-existent-log")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), ErrLogIDNotFound.Error())
}

// TestEmptyLogs verifies handling of empty log lists and empty stores
func TestEmptyLogs(t *testing.T) {
	helper, teardown := setupTest(t)
	defer teardown()

	// Test adding empty log list
	err := helper.store.AddLogs(helper.ctx, []telemetry.LogData{})
	assert.NoError(t, err)

	// Test getting logs from empty store
	logs, err := helper.store.GetLogs(helper.ctx)
	assert.NoError(t, err)
	assert.Empty(t, logs)
} 