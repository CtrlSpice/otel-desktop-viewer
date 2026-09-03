//go:build waterfallbench

package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
)

const (
	fixtureBaseTimestamp = uint64(1_700_000_000_000_000_000)
	fixtureTimestampGap  = uint64(10_000_000_000)
	missingParentOrdinal = uint64(999)

	benchmarkPayload64  = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	benchmarkPayload256 = benchmarkPayload64 + benchmarkPayload64 +
		benchmarkPayload64 + benchmarkPayload64
	benchmarkPayload1024 = benchmarkPayload256 + benchmarkPayload256 +
		benchmarkPayload256 + benchmarkPayload256
)

var fixtureServiceNames = [...]string{
	"edge-gateway",
	"checkout-api",
	"cart",
	"catalog",
	"inventory",
	"pricing",
	"payment",
	"fraud-detection",
	"orders",
	"shipping",
	"notifications",
	"postgres",
}

var fixtureScopeNames = [...]string{
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp",
	"github.com/jackc/pgx/v5",
	"waterfallbench.worker",
}

var fixtureScopeVersions = [...]string{"0.63.0", "5.7.5", "1.0.0"}

type fixtureSpanSpec struct {
	ordinal       uint64
	parentOrdinal uint64
	depth         int
	startOffset   uint64
}

type fixtureSpec struct {
	name                          string
	number                        byte
	spans                         []fixtureSpanSpec
	inputOrder                    []int
	serviceCount                  int
	scopeCount                    int
	expectedDisplayedSpanCount    int
	expectedMaximumDisplayedDepth int
	expectedFirstSpanOrdinal      uint64
	topology                      string
}

type fixtureManifest struct {
	SchemaVersion int                    `json:"schemaVersion"`
	Fixtures      []fixtureManifestEntry `json:"fixtures"`
}

type fixtureManifestEntry struct {
	Name                          string `json:"name"`
	Filename                      string `json:"filename"`
	SHA256                        string `json:"sha256"`
	Bytes                         int    `json:"bytes"`
	TraceID                       string `json:"traceId"`
	SpanCount                     int    `json:"spanCount"`
	ExpectedDisplayedSpanCount    int    `json:"expectedDisplayedSpanCount"`
	ExpectedMaximumDisplayedDepth int    `json:"expectedMaximumDisplayedDepth"`
	ExpectedFirstSpanID           string `json:"expectedFirstSpanId"`
	Topology                      string `json:"topology"`
}

type builtFixture struct {
	manifest fixtureManifestEntry
	data     []byte
}

type builtFixtureSet struct {
	fixtures []builtFixture
	manifest []byte
}

func buildFixtureSet() (builtFixtureSet, error) {
	specs := fixtureSpecs()
	built := builtFixtureSet{fixtures: make([]builtFixture, 0, len(specs))}
	manifest := fixtureManifest{SchemaVersion: 1, Fixtures: make([]fixtureManifestEntry, 0, len(specs))}

	for _, spec := range specs {
		data, err := marshalFixture(spec)
		if err != nil {
			return builtFixtureSet{}, fmt.Errorf("build fixture %q: %w", spec.name, err)
		}
		digest := sha256.Sum256(data)
		entry := fixtureManifestEntry{
			Name:                          spec.name,
			Filename:                      spec.name + ".otlp.pb",
			SHA256:                        hex.EncodeToString(digest[:]),
			Bytes:                         len(data),
			TraceID:                       traceIDHex(fixtureTraceID(spec.number)),
			SpanCount:                     len(spec.spans),
			ExpectedDisplayedSpanCount:    spec.expectedDisplayedSpanCount,
			ExpectedMaximumDisplayedDepth: spec.expectedMaximumDisplayedDepth,
			ExpectedFirstSpanID:           spanIDHexString(fixtureSpanID(spec.expectedFirstSpanOrdinal)),
			Topology:                      spec.topology,
		}
		built.fixtures = append(built.fixtures, builtFixture{manifest: entry, data: data})
		manifest.Fixtures = append(manifest.Fixtures, entry)
	}

	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return builtFixtureSet{}, fmt.Errorf("marshal fixture manifest: %w", err)
	}
	built.manifest = append(manifestBytes, '\n')
	return built, nil
}

func fixtureSpecs() []fixtureSpec {
	return []fixtureSpec{
		realisticFixtureSpec(),
		{
			name:                          "single-span",
			number:                        2,
			spans:                         []fixtureSpanSpec{{ordinal: 1}},
			inputOrder:                    []int{0},
			serviceCount:                  1,
			scopeCount:                    1,
			expectedDisplayedSpanCount:    1,
			expectedMaximumDisplayedDepth: 0,
			expectedFirstSpanOrdinal:      1,
			topology:                      "rooted-tree",
		},
		wideFixtureSpec(),
		deepFixtureSpec(),
		{
			name:   "multiple-roots",
			number: 5,
			spans: []fixtureSpanSpec{
				{ordinal: 1},
				{ordinal: 2, parentOrdinal: 1, depth: 1},
				{ordinal: 3},
				{ordinal: 4, parentOrdinal: 3, depth: 1},
				{ordinal: 5},
				{ordinal: 6, parentOrdinal: 5, depth: 1},
			},
			inputOrder:                    []int{5, 1, 3, 0, 4, 2},
			serviceCount:                  1,
			scopeCount:                    1,
			expectedDisplayedSpanCount:    6,
			expectedMaximumDisplayedDepth: 1,
			expectedFirstSpanOrdinal:      1,
			topology:                      "multiple-roots",
		},
		{
			name:   "orphan",
			number: 6,
			spans: []fixtureSpanSpec{
				{ordinal: 1, startOffset: 4_000_000},
				{ordinal: 2, parentOrdinal: 1, depth: 1},
				{ordinal: 3, parentOrdinal: missingParentOrdinal, startOffset: 1_000_000},
				{ordinal: 4, parentOrdinal: 3, depth: 1},
			},
			inputOrder:                    []int{3, 1, 2, 0},
			serviceCount:                  1,
			scopeCount:                    1,
			expectedDisplayedSpanCount:    4,
			expectedMaximumDisplayedDepth: 1,
			expectedFirstSpanOrdinal:      1,
			topology:                      "orphan",
		},
		{
			name:   "cycle",
			number: 7,
			spans: []fixtureSpanSpec{
				{ordinal: 1, parentOrdinal: 3},
				{ordinal: 2, parentOrdinal: 1, depth: 1},
				{ordinal: 3, parentOrdinal: 2, depth: 2},
			},
			inputOrder:                    []int{2, 0, 1},
			serviceCount:                  1,
			scopeCount:                    1,
			expectedDisplayedSpanCount:    3,
			expectedMaximumDisplayedDepth: 2,
			expectedFirstSpanOrdinal:      1,
			topology:                      "cycle",
		},
	}
}

func realisticFixtureSpec() fixtureSpec {
	levelWidths := [...]int{1, 4, 8, 12, 16, 20, 22, 20, 16, 12, 10, 8, 5, 3, 2}
	spans := make([]fixtureSpanSpec, 0, 159)
	previousLevelStart := 0
	previousLevelWidth := 0
	nextOrdinal := uint64(1)

	for depth, width := range levelWidths {
		levelStart := len(spans)
		for position := 0; position < width; position++ {
			parentOrdinal := uint64(0)
			if depth > 0 {
				parentPosition := position * previousLevelWidth / width
				parentOrdinal = spans[previousLevelStart+parentPosition].ordinal
			}
			spans = append(spans, fixtureSpanSpec{
				ordinal:       nextOrdinal,
				parentOrdinal: parentOrdinal,
				depth:         depth,
			})
			nextOrdinal++
		}
		previousLevelStart = levelStart
		previousLevelWidth = width
	}

	return fixtureSpec{
		name:                          "realistic-159",
		number:                        1,
		spans:                         spans,
		inputOrder:                    reverseOrder(len(spans)),
		serviceCount:                  len(fixtureServiceNames),
		scopeCount:                    len(fixtureScopeNames),
		expectedDisplayedSpanCount:    159,
		expectedMaximumDisplayedDepth: 14,
		expectedFirstSpanOrdinal:      1,
		topology:                      "rooted-tree",
	}
}

func wideFixtureSpec() fixtureSpec {
	spans := make([]fixtureSpanSpec, 0, 25)
	spans = append(spans, fixtureSpanSpec{ordinal: 1})
	for ordinal := uint64(2); ordinal <= 25; ordinal++ {
		spans = append(spans, fixtureSpanSpec{ordinal: ordinal, parentOrdinal: 1, depth: 1})
	}
	return fixtureSpec{
		name:                          "wide",
		number:                        3,
		spans:                         spans,
		inputOrder:                    reverseOrder(len(spans)),
		serviceCount:                  1,
		scopeCount:                    1,
		expectedDisplayedSpanCount:    25,
		expectedMaximumDisplayedDepth: 1,
		expectedFirstSpanOrdinal:      1,
		topology:                      "rooted-tree",
	}
}

func deepFixtureSpec() fixtureSpec {
	spans := make([]fixtureSpanSpec, 0, 18)
	for ordinal := uint64(1); ordinal <= 18; ordinal++ {
		parentOrdinal := uint64(0)
		if ordinal > 1 {
			parentOrdinal = ordinal - 1
		}
		spans = append(spans, fixtureSpanSpec{
			ordinal:       ordinal,
			parentOrdinal: parentOrdinal,
			depth:         int(ordinal - 1),
		})
	}
	return fixtureSpec{
		name:                          "deep",
		number:                        4,
		spans:                         spans,
		inputOrder:                    reverseOrder(len(spans)),
		serviceCount:                  1,
		scopeCount:                    1,
		expectedDisplayedSpanCount:    18,
		expectedMaximumDisplayedDepth: 17,
		expectedFirstSpanOrdinal:      1,
		topology:                      "rooted-tree",
	}
}

func reverseOrder(length int) []int {
	order := make([]int, length)
	for i := range order {
		order[i] = length - i - 1
	}
	return order
}

func marshalFixture(spec fixtureSpec) ([]byte, error) {
	if err := validateFixtureSpec(spec); err != nil {
		return nil, err
	}

	traces := ptrace.NewTraces()
	resources := traces.ResourceSpans()
	for serviceIndex := 0; serviceIndex < spec.serviceCount; serviceIndex++ {
		resourceSpans := resources.AppendEmpty()
		populateFixtureResource(resourceSpans, serviceIndex)
		for scopeIndex := 0; scopeIndex < spec.scopeCount; scopeIndex++ {
			scopeSpans := resourceSpans.ScopeSpans().AppendEmpty()
			populateFixtureScope(scopeSpans, scopeIndex)
		}
	}

	for _, logicalIndex := range spec.inputOrder {
		spanSpec := spec.spans[logicalIndex]
		serviceIndex := logicalIndex % spec.serviceCount
		scopeIndex := (logicalIndex / spec.serviceCount) % spec.scopeCount
		span := resources.At(serviceIndex).ScopeSpans().At(scopeIndex).Spans().AppendEmpty()
		populateFixtureSpan(span, spec, spanSpec)
	}

	request := ptraceotlp.NewExportRequestFromTraces(traces)
	data, err := request.MarshalProto()
	if err != nil {
		return nil, fmt.Errorf("marshal OTLP export request: %w", err)
	}
	return data, nil
}

func validateFixtureSpec(spec fixtureSpec) error {
	if spec.name == "" || spec.number == 0 {
		return fmt.Errorf("name and fixture number must be set")
	}
	if spec.serviceCount < 1 || spec.serviceCount > len(fixtureServiceNames) {
		return fmt.Errorf("invalid service count %d", spec.serviceCount)
	}
	if spec.scopeCount < 1 || spec.scopeCount > len(fixtureScopeNames) {
		return fmt.Errorf("invalid scope count %d", spec.scopeCount)
	}
	if len(spec.inputOrder) != len(spec.spans) {
		return fmt.Errorf("input order has %d entries for %d spans", len(spec.inputOrder), len(spec.spans))
	}
	if spec.expectedFirstSpanOrdinal == 0 {
		return fmt.Errorf("expected first span ordinal must be set")
	}
	seen := make([]bool, len(spec.spans))
	for _, index := range spec.inputOrder {
		if index < 0 || index >= len(spec.spans) {
			return fmt.Errorf("input order index %d is out of range", index)
		}
		if seen[index] {
			return fmt.Errorf("input order index %d is duplicated", index)
		}
		seen[index] = true
	}
	return nil
}

func populateFixtureResource(resourceSpans ptrace.ResourceSpans, serviceIndex int) {
	resourceSpans.SetSchemaUrl("https://opentelemetry.io/schemas/1.37.0")
	resource := resourceSpans.Resource()
	attrs := resource.Attributes()
	attrs.PutStr("service.name", fixtureServiceNames[serviceIndex])
	attrs.PutStr("service.namespace", "shop")
	attrs.PutStr("service.instance.id", fmt.Sprintf("%s-%02d", fixtureServiceNames[serviceIndex], serviceIndex+1))
	attrs.PutStr("deployment.environment.name", "benchmark")
	attrs.PutStr("telemetry.sdk.language", "go")
	attrs.PutInt("service.replica", int64(serviceIndex%3+1))
	attrs.PutBool("benchmark.synthetic", true)
	zones := attrs.PutEmptySlice("benchmark.zones")
	zones.AppendEmpty().SetStr("zone-a")
	zones.AppendEmpty().SetStr("zone-b")
	if serviceIndex == 0 {
		resource.SetDroppedAttributesCount(1)
	}
}

func populateFixtureScope(scopeSpans ptrace.ScopeSpans, scopeIndex int) {
	scopeSpans.SetSchemaUrl("https://opentelemetry.io/schemas/1.37.0")
	scope := scopeSpans.Scope()
	scope.SetName(fixtureScopeNames[scopeIndex])
	scope.SetVersion(fixtureScopeVersions[scopeIndex])
	attrs := scope.Attributes()
	attrs.PutStr("scope.role", scopeRole(scopeIndex))
	attrs.PutInt("scope.index", int64(scopeIndex))
	attrs.PutBool("scope.stable", true)
	if scopeIndex == 0 {
		scope.SetDroppedAttributesCount(1)
	}
}

func populateFixtureSpan(span ptrace.Span, fixture fixtureSpec, spec fixtureSpanSpec) {
	traceID := fixtureTraceID(fixture.number)
	span.SetTraceID(traceID)
	span.SetSpanID(fixtureSpanID(spec.ordinal))
	if spec.parentOrdinal != 0 {
		span.SetParentSpanID(fixtureSpanID(spec.parentOrdinal))
	}
	span.TraceState().FromRaw(fmt.Sprintf("vendor=fixture%02d", fixture.number))
	span.SetName(spanOperation(spec.ordinal))
	span.SetKind(fixtureSpanKind(spec.ordinal))
	span.SetFlags(1 | uint32(spec.ordinal%2)<<8)

	startOffset := uint64(spec.depth)*100_000_000 + spec.ordinal*1_000_000
	if spec.startOffset != 0 {
		startOffset = spec.startOffset
	}
	start := fixtureBaseTimestamp + uint64(fixture.number)*fixtureTimestampGap + startOffset
	duration := uint64(2_500_000_000) - uint64(spec.depth)*120_000_000 + spec.ordinal%11*1_000_000
	span.SetStartTimestamp(pcommon.Timestamp(start))
	span.SetEndTimestamp(pcommon.Timestamp(start + duration))

	attrs := span.Attributes()
	attrs.PutStr("http.request.method", fixtureHTTPMethod(spec.ordinal))
	attrs.PutStr("http.route", fixtureHTTPRoute(spec.ordinal))
	attrs.PutInt("http.response.status_code", int64(200+spec.ordinal%5))
	attrs.PutBool("cache.hit", spec.ordinal%3 == 0)
	attrs.PutDouble("benchmark.load_factor", float64(spec.ordinal%17+1)/10)
	attrs.PutStr("benchmark.payload", fixturePayload(fixture.number, spec.ordinal))
	payloadBytes := attrs.PutEmptyBytes("benchmark.payload_bytes")
	payloadBytes.Append(0x77, 0x61, 0x74, 0x65, 0x72, byte(spec.ordinal>>8), byte(spec.ordinal))
	tags := attrs.PutEmptySlice("benchmark.tags")
	tags.AppendEmpty().SetStr("fixture")
	tags.AppendEmpty().SetStr("waterfall")
	peer := attrs.PutEmptyMap("peer")
	peer.PutStr("region", "us-central1")
	peer.PutInt("shard", int64(spec.ordinal%8))

	if spec.ordinal%13 == 0 {
		span.SetDroppedAttributesCount(2)
	}
	if spec.ordinal%17 == 0 {
		span.SetDroppedEventsCount(1)
	}
	if spec.ordinal%19 == 0 {
		span.SetDroppedLinksCount(1)
	}

	setFixtureStatus(span.Status(), fixture.number, spec.ordinal)
	appendFixtureEvent(span, spec.ordinal, start, 1)
	if spec.ordinal%7 == 0 {
		appendFixtureEvent(span, spec.ordinal, start, 2)
	}
	if spec.ordinal == 1 {
		appendFixtureLink(span, fixture.number, spec.ordinal, 1)
		appendFixtureLink(span, fixture.number, spec.ordinal, 2)
	} else if spec.ordinal%23 == 0 {
		appendFixtureLink(span, fixture.number, spec.ordinal, 1)
	}
}

func appendFixtureEvent(span ptrace.Span, ordinal, start uint64, eventIndex uint64) {
	event := span.Events().AppendEmpty()
	event.SetName(fmt.Sprintf("event-%03d-%d", ordinal, eventIndex))
	event.SetTimestamp(pcommon.Timestamp(start + eventIndex*100_000))
	attrs := event.Attributes()
	attrs.PutInt("event.sequence", int64(ordinal*10+eventIndex))
	attrs.PutStr("event.phase", eventPhase(eventIndex))
	attrs.PutBool("event.retry", eventIndex == 2)
	if ordinal%11 == 0 && eventIndex == 1 {
		event.SetDroppedAttributesCount(1)
	}
}

func appendFixtureLink(span ptrace.Span, fixtureNumber byte, ordinal, linkIndex uint64) {
	link := span.Links().AppendEmpty()
	link.SetTraceID(fixtureLinkTraceID(fixtureNumber, ordinal, linkIndex))
	link.SetSpanID(fixtureLinkSpanID(fixtureNumber, ordinal, linkIndex))
	link.TraceState().FromRaw(fmt.Sprintf("vendor=link%02d", fixtureNumber))
	link.SetFlags(1 | uint32(linkIndex)<<8)
	attrs := link.Attributes()
	attrs.PutStr("link.reason", linkReason(linkIndex))
	attrs.PutInt("link.sequence", int64(ordinal*10+linkIndex))
	attrs.PutBool("link.remote", true)
	if ordinal == 1 && linkIndex == 2 {
		link.SetDroppedAttributesCount(1)
	}
}

func setFixtureStatus(status ptrace.Status, fixtureNumber byte, ordinal uint64) {
	if fixtureNumber == 1 && (ordinal == 23 || ordinal == 88 || ordinal == 147) {
		status.SetCode(ptrace.StatusCodeError)
		status.SetMessage("synthetic upstream failure")
		return
	}
	if ordinal%4 == 0 {
		status.SetCode(ptrace.StatusCodeOk)
		return
	}
	status.SetCode(ptrace.StatusCodeUnset)
}

func fixtureTraceID(number byte) pcommon.TraceID {
	return pcommon.TraceID{
		0x57, 0x41, 0x54, 0x45, 0x52, 0x46, 0x41, 0x4c,
		0x4c, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, number,
	}
}

func fixtureSpanID(ordinal uint64) pcommon.SpanID {
	var id pcommon.SpanID
	binary.BigEndian.PutUint64(id[:], ordinal)
	return id
}

func fixtureLinkTraceID(fixtureNumber byte, ordinal, linkIndex uint64) pcommon.TraceID {
	return pcommon.TraceID{
		0x4c, 0x49, 0x4e, 0x4b, 0x54, 0x52, 0x41, 0x43,
		0x45, 0x00, 0x00, fixtureNumber, byte(ordinal >> 16), byte(ordinal >> 8), byte(ordinal), byte(linkIndex),
	}
}

func fixtureLinkSpanID(fixtureNumber byte, ordinal, linkIndex uint64) pcommon.SpanID {
	return fixtureSpanID(0x8000_0000_0000_0000 | uint64(fixtureNumber)<<48 | ordinal<<8 | linkIndex)
}

func traceIDHex(id pcommon.TraceID) string {
	return hex.EncodeToString(id[:])
}

func spanIDHexString(id pcommon.SpanID) string {
	return hex.EncodeToString(id[:])
}

func fixturePayload(fixtureNumber byte, ordinal uint64) string {
	if fixtureNumber != 1 {
		return benchmarkPayload64
	}
	switch (ordinal - 1) % 3 {
	case 0:
		return benchmarkPayload64
	case 1:
		return benchmarkPayload256
	default:
		return benchmarkPayload1024
	}
}

func fixtureSpanKind(ordinal uint64) ptrace.SpanKind {
	switch (ordinal - 1) % 6 {
	case 0:
		return ptrace.SpanKindUnspecified
	case 1:
		return ptrace.SpanKindInternal
	case 2:
		return ptrace.SpanKindServer
	case 3:
		return ptrace.SpanKindClient
	case 4:
		return ptrace.SpanKindProducer
	default:
		return ptrace.SpanKindConsumer
	}
}

func spanOperation(ordinal uint64) string {
	switch ordinal % 8 {
	case 0:
		return "POST /checkout"
	case 1:
		return "GET /catalog/{sku}"
	case 2:
		return "reserve inventory"
	case 3:
		return "calculate price"
	case 4:
		return "authorize payment"
	case 5:
		return "INSERT orders"
	case 6:
		return "publish order.created"
	default:
		return "send confirmation"
	}
}

func fixtureHTTPMethod(ordinal uint64) string {
	switch ordinal % 4 {
	case 0:
		return "POST"
	case 1:
		return "GET"
	case 2:
		return "PUT"
	default:
		return "DELETE"
	}
}

func fixtureHTTPRoute(ordinal uint64) string {
	switch ordinal % 4 {
	case 0:
		return "/checkout"
	case 1:
		return "/catalog/{sku}"
	case 2:
		return "/inventory/{sku}"
	default:
		return "/cart/{id}"
	}
}

func scopeRole(scopeIndex int) string {
	switch scopeIndex {
	case 0:
		return "http"
	case 1:
		return "database"
	default:
		return "worker"
	}
}

func eventPhase(eventIndex uint64) string {
	if eventIndex == 1 {
		return "request.accepted"
	}
	return "request.retried"
}

func linkReason(linkIndex uint64) string {
	if linkIndex == 1 {
		return "batch predecessor"
	}
	return "async continuation"
}
