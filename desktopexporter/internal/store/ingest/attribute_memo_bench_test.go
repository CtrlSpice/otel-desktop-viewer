package ingest_test

import (
	"fmt"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

// captureShapedSets mirrors the reference capture: 89 distinct five-label sets,
// which 294,607 datapoints resolve to.
func captureShapedSets(n int) []pcommon.Map {
	sets := make([]pcommon.Map, n)
	for i := range n {
		m := pcommon.NewMap()
		m.PutStr("driver", fmt.Sprintf("D%02d", i))
		m.PutStr("team", fmt.Sprintf("T%02d", i%10))
		m.PutStr("session", "race")
		m.PutInt("lap", int64(i))
		m.PutStr("compound", "medium")
		sets[i] = m
	}
	return sets
}

// BenchmarkAttributeSetCaptureShape is the shape the change is justified by:
// the same 89 sets reporting over and over, which is what a metrics session is.
func BenchmarkAttributeSetCaptureShape(b *testing.B) {
	sets := captureShapedSets(89)
	ingest.ResetAttributeMemo()
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		ingest.AttributeSet(sets[i%len(sets)], ingest.ScopeDatapoint)
		i++
	}
	b.StopTimer()
	hits, misses, _ := ingest.AttributeMemoStats()
	if hits+misses > 0 {
		b.ReportMetric(100*float64(hits)/float64(hits+misses), "%hit")
	}
}

// BenchmarkAttributeSetAllDistinct is the adversarial shape: a label the memo
// can never hit, so this measures what the lookup costs when it always misses.
func BenchmarkAttributeSetAllDistinct(b *testing.B) {
	sets := captureShapedSets(20000)
	ingest.ResetAttributeMemo()
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		ingest.AttributeSet(sets[i%len(sets)], ingest.ScopeDatapoint)
		i++
	}
}
