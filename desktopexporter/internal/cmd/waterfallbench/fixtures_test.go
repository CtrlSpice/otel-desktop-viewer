//go:build waterfallbench

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"flag"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
)

var updateFixtures = flag.Bool("update-fixtures", false, "rewrite the deterministic OTLP fixture files and manifest")

var checkedFixtureContracts = []struct {
	name           string
	filename       string
	traceID        string
	spanCount      int
	displayedCount int
	maximumDepth   int
	firstSpanID    string
	topology       string
	rootCount      int
	orphanCount    int
	cycleNodeCount int
}{
	{"realistic-159", "realistic-159.otlp.pb", "574154455246414c4c00000000000001", 159, 159, 14, "0000000000000001", "rooted-tree", 1, 0, 0},
	{"single-span", "single-span.otlp.pb", "574154455246414c4c00000000000002", 1, 1, 0, "0000000000000001", "rooted-tree", 1, 0, 0},
	{"wide", "wide.otlp.pb", "574154455246414c4c00000000000003", 25, 25, 1, "0000000000000001", "rooted-tree", 1, 0, 0},
	{"deep", "deep.otlp.pb", "574154455246414c4c00000000000004", 18, 18, 17, "0000000000000001", "rooted-tree", 1, 0, 0},
	{"multiple-roots", "multiple-roots.otlp.pb", "574154455246414c4c00000000000005", 6, 6, 1, "0000000000000001", "multiple-roots", 3, 0, 0},
	{"orphan", "orphan.otlp.pb", "574154455246414c4c00000000000006", 4, 4, 1, "0000000000000001", "orphan", 1, 1, 0},
	{"cycle", "cycle.otlp.pb", "574154455246414c4c00000000000007", 3, 3, 2, "0000000000000001", "cycle", 0, 0, 3},
}

// Regenerate with:
// go test -tags=waterfallbench ./internal/cmd/waterfallbench -run TestFixtureGoldens -update-fixtures
func TestFixtureGoldens(t *testing.T) {
	first, err := buildFixtureSet()
	require.NoError(t, err)
	second, err := buildFixtureSet()
	require.NoError(t, err)

	require.Len(t, second.fixtures, len(first.fixtures))
	for i := range first.fixtures {
		require.Equal(t, first.fixtures[i].manifest, second.fixtures[i].manifest,
			"fixture %q metadata changed between builds", first.fixtures[i].manifest.Name)
		require.Equal(t, first.fixtures[i].data, second.fixtures[i].data,
			"fixture %q bytes changed between builds", first.fixtures[i].manifest.Name)
	}
	require.Equal(t, first.manifest, second.manifest, "manifest changed between builds")

	if *updateFixtures {
		require.NoError(t, os.MkdirAll("testdata", 0o755))
		for _, fixture := range first.fixtures {
			path := filepath.Join("testdata", fixture.manifest.Filename)
			require.NoError(t, writeFileAtomically(path, fixture.data))
		}
		// Move the manifest into place only after every body is complete.
		require.NoError(t, writeFileAtomically(filepath.Join("testdata", "manifest.json"), first.manifest))
	}

	checkedManifest, err := os.ReadFile(filepath.Join("testdata", "manifest.json"))
	require.NoError(t, err, "missing fixture manifest; run with -update-fixtures")
	require.Equal(t, first.manifest, checkedManifest,
		"fixture manifest changed; if deliberate, re-run with -update-fixtures and review every digest")
	for _, fixture := range first.fixtures {
		path := filepath.Join("testdata", fixture.manifest.Filename)
		checked, err := os.ReadFile(path)
		require.NoError(t, err, "missing fixture; run with -update-fixtures")
		require.Equal(t, fixture.data, checked,
			"fixture %q changed; if deliberate, re-run with -update-fixtures and review its manifest entry", fixture.manifest.Name)
	}

	entries, err := os.ReadDir("testdata")
	require.NoError(t, err)
	actualFiles := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			actualFiles = append(actualFiles, entry.Name())
		}
	}
	expectedFiles := []string{"manifest.json"}
	for _, fixture := range first.fixtures {
		expectedFiles = append(expectedFiles, fixture.manifest.Filename)
	}
	sort.Strings(actualFiles)
	sort.Strings(expectedFiles)
	require.Equal(t, expectedFiles, actualFiles, "testdata contains an unmanifested fixture or is missing a fixture")
}

func writeFileAtomically(path string, data []byte) (err error) {
	temporary, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	closed := false
	defer func() {
		if !closed {
			_ = temporary.Close()
		}
		if err != nil {
			_ = os.Remove(temporaryPath)
		}
	}()

	written, err := temporary.Write(data)
	if err != nil {
		return err
	}
	if written != len(data) {
		return io.ErrShortWrite
	}
	if err = temporary.Chmod(0o644); err != nil {
		return err
	}
	err = temporary.Close()
	closed = true
	if err != nil {
		return err
	}
	err = os.Rename(temporaryPath, path)
	return err
}

func TestCheckedFixtureContracts(t *testing.T) {
	manifestBytes, err := os.ReadFile(filepath.Join("testdata", "manifest.json"))
	require.NoError(t, err)
	require.True(t, bytes.HasSuffix(manifestBytes, []byte{'\n'}), "manifest must end in LF")
	require.NotContains(t, manifestBytes, []byte{'\r'}, "manifest must use LF, not CRLF")
	require.NotContains(t, strings.ToLower(string(manifestBytes)), `"timestamp"`, "manifest must not contain a generated timestamp")

	manifest := decodeManifest(t, manifestBytes)
	require.Equal(t, 1, manifest.SchemaVersion)
	require.Len(t, manifest.Fixtures, len(checkedFixtureContracts))

	for i, contract := range checkedFixtureContracts {
		entry := manifest.Fixtures[i]
		t.Run(contract.name, func(t *testing.T) {
			require.Equal(t, contract.name, entry.Name)
			require.Equal(t, contract.filename, entry.Filename)
			require.Equal(t, contract.traceID, entry.TraceID)
			require.Equal(t, contract.spanCount, entry.SpanCount)
			require.Equal(t, contract.displayedCount, entry.ExpectedDisplayedSpanCount)
			require.Equal(t, contract.maximumDepth, entry.ExpectedMaximumDisplayedDepth)
			require.Equal(t, contract.firstSpanID, entry.ExpectedFirstSpanID)
			require.Equal(t, contract.topology, entry.Topology)
			require.Regexp(t, `^[0-9a-f]{64}$`, entry.SHA256)

			data, err := os.ReadFile(filepath.Join("testdata", entry.Filename))
			require.NoError(t, err)
			require.Equal(t, len(data), entry.Bytes)
			digest := sha256.Sum256(data)
			require.Equal(t, hex.EncodeToString(digest[:]), entry.SHA256)

			// Each operation gets a fresh decode. pdata values are reference types;
			// sharing one request between consumers would let a future mutation make
			// an unrelated assertion observe changed data.
			remarshalRequest := decodeExportRequest(t, data)
			remarshaled, err := remarshalRequest.MarshalProto()
			require.NoError(t, err)
			require.Equal(t, data, remarshaled, "decode/remarshal changed the OTLP request body")

			observationRequest := decodeExportRequest(t, data)
			observation := observeFixture(t, observationRequest)
			require.Equal(t, contract.spanCount, len(observation.spans))
			require.Equal(t, map[string]struct{}{contract.traceID: {}}, observation.traceIDs)
			assertSequentialSpanIDs(t, observation, contract.spanCount)
			assertFixtureCoverage(t, contract.name, observation)

			topology := analyzeTopology(t, observation)
			require.Len(t, topology.roots, contract.rootCount)
			require.Len(t, topology.orphans, contract.orphanCount)
			require.Len(t, topology.cycleNodes, contract.cycleNodeCount)
			require.Equal(t, contract.topology, classifyTopology(topology))
			require.Len(t, topology.depthByID, contract.displayedCount)
			require.Equal(t, contract.maximumDepth, topology.maximumDepth)
			require.NotEmpty(t, topology.displayRoots)
			require.Equal(t, contract.firstSpanID, topology.displayRoots[0])
			assertUniqueSiblingAndDisplayRootStarts(t, observation, topology)
			assertFixtureSpecificTopology(t, contract.name, observation, topology)
		})
	}
}

func TestLoadFixture(t *testing.T) {
	for _, contract := range checkedFixtureContracts {
		t.Run(contract.name, func(t *testing.T) {
			entry, data, err := loadFixture(contract.name)
			require.NoError(t, err)
			require.Equal(t, contract.name, entry.Name)
			require.Equal(t, contract.filename, entry.Filename)
			require.Equal(t, contract.traceID, entry.TraceID)
			require.Equal(t, contract.spanCount, entry.SpanCount)
			require.Equal(t, contract.displayedCount, entry.ExpectedDisplayedSpanCount)
			require.Equal(t, contract.maximumDepth, entry.ExpectedMaximumDisplayedDepth)
			require.Equal(t, contract.firstSpanID, entry.ExpectedFirstSpanID)
			require.Equal(t, contract.topology, entry.Topology)
			require.Len(t, data, entry.Bytes)
			digest := sha256.Sum256(data)
			require.Equal(t, entry.SHA256, hex.EncodeToString(digest[:]))

			checked, err := os.ReadFile(filepath.Join("testdata", contract.filename))
			require.NoError(t, err)
			require.Equal(t, checked, data)
		})
	}
}

func TestLoadFixtureRejectsUnknownName(t *testing.T) {
	entry, data, err := loadFixture("not-a-fixture")
	require.ErrorIs(t, err, errFixtureNotFound)
	require.Equal(t, fixtureManifestEntry{}, entry)
	require.Nil(t, data)
}

func TestLoadFixtureReturnsFreshBytes(t *testing.T) {
	_, first, err := loadFixture("single-span")
	require.NoError(t, err)
	_, second, err := loadFixture("single-span")
	require.NoError(t, err)
	require.NotEmpty(t, first)
	require.Equal(t, first, second)

	want := bytes.Clone(second)
	first[0] ^= 0xff
	require.Equal(t, want, second, "mutating one load changed another returned byte slice")

	_, third, err := loadFixture("single-span")
	require.NoError(t, err)
	require.Equal(t, want, third, "mutating one load changed the embedded fixture")
}

func TestExperimentPinsHeadlineFixture(t *testing.T) {
	experimentPath := filepath.Join("..", "..", "..", "..", "benchmarks", "trace-waterfall", "experiment.json")
	data, err := os.ReadFile(experimentPath)
	require.NoError(t, err)

	var experiment struct {
		Status   string `json:"status"`
		Fixtures struct {
			Manifest string `json:"manifest"`
			Headline struct {
				Name   string `json:"name"`
				SHA256 string `json:"sha256"`
			} `json:"headline"`
		} `json:"fixtures"`
	}
	require.NoError(t, json.Unmarshal(data, &experiment))
	require.Equal(t, "phase-2-complete", experiment.Status)
	require.Equal(t, "../../desktopexporter/internal/cmd/waterfallbench/testdata/manifest.json", experiment.Fixtures.Manifest)
	require.Equal(t, "realistic-159", experiment.Fixtures.Headline.Name)

	manifestData, err := os.ReadFile(filepath.Join("testdata", "manifest.json"))
	require.NoError(t, err)
	manifest := decodeManifest(t, manifestData)
	require.NotEmpty(t, manifest.Fixtures)
	require.Equal(t, manifest.Fixtures[0].SHA256, experiment.Fixtures.Headline.SHA256)

	var rawExperiment struct {
		Fixtures struct {
			Headline map[string]json.RawMessage `json:"headline"`
		} `json:"fixtures"`
	}
	require.NoError(t, json.Unmarshal(data, &rawExperiment))
	require.Len(t, rawExperiment.Fixtures.Headline, 2,
		"headline metadata belongs in the manifest; experiment.json should pin only its name and digest")
	require.Contains(t, rawExperiment.Fixtures.Headline, "name")
	require.Contains(t, rawExperiment.Fixtures.Headline, "sha256")
}

func TestBenchmarkGoFilesUseWaterfallBuildTag(t *testing.T) {
	entries, err := os.ReadDir(".")
	require.NoError(t, err)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" {
			continue
		}
		data, err := os.ReadFile(entry.Name())
		require.NoError(t, err)
		require.Truef(t, bytes.HasPrefix(data, []byte("//go:build waterfallbench\n")),
			"%s must remain excluded unless the waterfallbench tag is set", entry.Name())
	}
}

type observedSpan struct {
	traceID    string
	id         string
	parentID   string
	ordinal    uint64
	start      uint64
	end        uint64
	eventTimes []uint64
}

type fixtureObservation struct {
	spans                     []observedSpan
	envelopeOrdinals          []uint64
	traceIDs                  map[string]struct{}
	services                  map[string]struct{}
	scopes                    map[string]struct{}
	kinds                     map[ptrace.SpanKind]struct{}
	statusCodes               map[ptrace.StatusCode]struct{}
	eventNames                map[string]struct{}
	eventTimestamps           map[uint64]struct{}
	linkTargets               map[string]struct{}
	payloadSizes              map[int]int
	resourceCount             int
	scopeEnvelopeCount        int
	eventCount                int
	linkCount                 int
	maximumLinksOnSpan        int
	errorCount                int
	resourceAttributeOwners   int
	scopeAttributeOwners      int
	spanAttributeOwners       int
	eventAttributeOwners      int
	linkAttributeOwners       int
	spanTraceStateCount       int
	spanFlagsPresent          bool
	linkFlagsPresent          bool
	resourceSchemaPresent     bool
	scopeSchemaPresent        bool
	resourceDroppedAttributes bool
	scopeDroppedAttributes    bool
	spanDroppedAttributes     bool
	spanDroppedEvents         bool
	spanDroppedLinks          bool
	eventDroppedAttributes    bool
	linkDroppedAttributes     bool
}

func observeFixture(t *testing.T, request ptraceotlp.ExportRequest) fixtureObservation {
	t.Helper()
	observation := fixtureObservation{
		traceIDs:        make(map[string]struct{}),
		services:        make(map[string]struct{}),
		scopes:          make(map[string]struct{}),
		kinds:           make(map[ptrace.SpanKind]struct{}),
		statusCodes:     make(map[ptrace.StatusCode]struct{}),
		eventNames:      make(map[string]struct{}),
		eventTimestamps: make(map[uint64]struct{}),
		linkTargets:     make(map[string]struct{}),
		payloadSizes:    make(map[int]int),
	}
	compositeKeys := make(map[string]struct{})

	for _, resourceSpans := range request.Traces().ResourceSpans().All() {
		observation.resourceCount++
		resource := resourceSpans.Resource()
		if resource.Attributes().Len() > 0 {
			observation.resourceAttributeOwners++
		}
		assertFiniteAttributeValue(t, resource.Attributes().AsRaw())
		service, ok := resource.Attributes().Get("service.name")
		require.True(t, ok, "every resource must identify its service")
		require.NotEmpty(t, service.Str())
		observation.services[service.Str()] = struct{}{}
		observation.resourceSchemaPresent = observation.resourceSchemaPresent || resourceSpans.SchemaUrl() != ""
		observation.resourceDroppedAttributes = observation.resourceDroppedAttributes || resource.DroppedAttributesCount() > 0
		require.Greater(t, resourceSpans.ScopeSpans().Len(), 0, "every resource envelope must contain spans")

		for _, scopeSpans := range resourceSpans.ScopeSpans().All() {
			observation.scopeEnvelopeCount++
			require.Greater(t, scopeSpans.Spans().Len(), 0, "every scope envelope must contain spans")
			scope := scopeSpans.Scope()
			require.NotEmpty(t, scope.Name())
			observation.scopes[scope.Name()] = struct{}{}
			if scope.Attributes().Len() > 0 {
				observation.scopeAttributeOwners++
			}
			assertFiniteAttributeValue(t, scope.Attributes().AsRaw())
			observation.scopeSchemaPresent = observation.scopeSchemaPresent || scopeSpans.SchemaUrl() != ""
			observation.scopeDroppedAttributes = observation.scopeDroppedAttributes || scope.DroppedAttributesCount() > 0

			for _, span := range scopeSpans.Spans().All() {
				traceID := traceIDHex(span.TraceID())
				spanID := spanIDHex(span.SpanID())
				require.NotEqual(t, strings.Repeat("0", 32), traceID, "trace ID must be nonzero")
				require.NotEqual(t, strings.Repeat("0", 16), spanID, "span ID must be nonzero")
				key := traceID + "/" + spanID
				_, duplicate := compositeKeys[key]
				require.False(t, duplicate, "duplicate trace/span composite key %s", key)
				compositeKeys[key] = struct{}{}
				observation.traceIDs[traceID] = struct{}{}

				parentID := ""
				if span.ParentSpanID() != (pcommon.SpanID{}) {
					parentID = spanIDHex(span.ParentSpanID())
				}
				start := uint64(span.StartTimestamp())
				end := uint64(span.EndTimestamp())
				require.GreaterOrEqual(t, end, start)
				rawSpanID := span.SpanID()
				observed := observedSpan{
					traceID:  traceID,
					id:       spanID,
					parentID: parentID,
					ordinal:  binary.BigEndian.Uint64(rawSpanID[:]),
					start:    start,
					end:      end,
				}
				observation.envelopeOrdinals = append(observation.envelopeOrdinals, observed.ordinal)

				if span.Attributes().Len() > 0 {
					observation.spanAttributeOwners++
				}
				assertFiniteAttributeValue(t, span.Attributes().AsRaw())
				payload, ok := span.Attributes().Get("benchmark.payload")
				require.True(t, ok, "every span must contain benchmark.payload")
				switch payload.Str() {
				case benchmarkPayload64, benchmarkPayload256, benchmarkPayload1024:
					observation.payloadSizes[len(payload.Str())]++
				default:
					t.Fatalf("unexpected benchmark.payload with %d bytes", len(payload.Str()))
				}
				observation.kinds[span.Kind()] = struct{}{}
				observation.statusCodes[span.Status().Code()] = struct{}{}
				if span.Status().Code() == ptrace.StatusCodeError {
					observation.errorCount++
				}
				observation.spanTraceStateCount += boolInt(span.TraceState().AsRaw() != "")
				observation.spanFlagsPresent = observation.spanFlagsPresent || span.Flags() != 0
				observation.spanDroppedAttributes = observation.spanDroppedAttributes || span.DroppedAttributesCount() > 0
				observation.spanDroppedEvents = observation.spanDroppedEvents || span.DroppedEventsCount() > 0
				observation.spanDroppedLinks = observation.spanDroppedLinks || span.DroppedLinksCount() > 0

				for _, event := range span.Events().All() {
					observation.eventCount++
					_, duplicateName := observation.eventNames[event.Name()]
					require.False(t, duplicateName, "event name %q is not unique", event.Name())
					observation.eventNames[event.Name()] = struct{}{}
					timestamp := uint64(event.Timestamp())
					_, duplicateTimestamp := observation.eventTimestamps[timestamp]
					require.False(t, duplicateTimestamp, "event timestamp %d is not unique", timestamp)
					observation.eventTimestamps[timestamp] = struct{}{}
					require.GreaterOrEqual(t, timestamp, start)
					require.LessOrEqual(t, timestamp, end)
					observed.eventTimes = append(observed.eventTimes, timestamp)
					if event.Attributes().Len() > 0 {
						observation.eventAttributeOwners++
					}
					assertFiniteAttributeValue(t, event.Attributes().AsRaw())
					observation.eventDroppedAttributes = observation.eventDroppedAttributes || event.DroppedAttributesCount() > 0
				}

				linkCount := 0
				for _, link := range span.Links().All() {
					linkCount++
					observation.linkCount++
					require.NotEqual(t, pcommon.TraceID{}, link.TraceID(), "link trace ID must be nonzero")
					require.NotEqual(t, pcommon.SpanID{}, link.SpanID(), "link span ID must be nonzero")
					target := traceIDHex(link.TraceID()) + "/" + spanIDHex(link.SpanID())
					_, duplicateTarget := observation.linkTargets[target]
					require.False(t, duplicateTarget, "duplicate linked trace/span composite key %s", target)
					observation.linkTargets[target] = struct{}{}
					if link.Attributes().Len() > 0 {
						observation.linkAttributeOwners++
					}
					assertFiniteAttributeValue(t, link.Attributes().AsRaw())
					observation.linkFlagsPresent = observation.linkFlagsPresent || link.Flags() != 0
					observation.linkDroppedAttributes = observation.linkDroppedAttributes || link.DroppedAttributesCount() > 0
				}
				observation.maximumLinksOnSpan = max(observation.maximumLinksOnSpan, linkCount)
				observation.spans = append(observation.spans, observed)
			}
		}
	}
	return observation
}

type fixtureTopology struct {
	roots        []string
	orphans      []string
	cycleNodes   map[string]struct{}
	displayRoots []string
	depthByID    map[string]int
	children     map[string][]string
	maximumDepth int
}

func analyzeTopology(t *testing.T, observation fixtureObservation) fixtureTopology {
	t.Helper()
	byID := make(map[string]observedSpan, len(observation.spans))
	for _, span := range observation.spans {
		byID[span.id] = span
	}

	topology := fixtureTopology{
		cycleNodes:   make(map[string]struct{}),
		depthByID:    make(map[string]int),
		children:     make(map[string][]string),
		maximumDepth: -1,
	}
	for _, span := range observation.spans {
		switch {
		case span.parentID == "":
			topology.roots = append(topology.roots, span.id)
		case !hasSpan(byID, span.parentID):
			topology.orphans = append(topology.orphans, span.id)
		}
		if span.parentID != "" {
			topology.children[span.parentID] = append(topology.children[span.parentID], span.id)
		}
	}

	done := make(map[string]bool, len(observation.spans))
	for _, start := range observation.spans {
		if done[start.id] {
			continue
		}
		path := make([]string, 0)
		pathIndex := make(map[string]int)
		current := start.id
		for {
			if cycleStart, ok := pathIndex[current]; ok {
				for _, id := range path[cycleStart:] {
					topology.cycleNodes[id] = struct{}{}
				}
				break
			}
			if done[current] {
				break
			}
			span, ok := byID[current]
			if !ok || span.parentID == "" || !hasSpan(byID, span.parentID) {
				break
			}
			pathIndex[current] = len(path)
			path = append(path, current)
			current = span.parentID
		}
		for _, id := range path {
			done[id] = true
		}
	}

	less := func(left, right string) bool {
		if byID[left].start != byID[right].start {
			return byID[left].start < byID[right].start
		}
		return left < right
	}
	sort.Slice(topology.roots, func(i, j int) bool { return less(topology.roots[i], topology.roots[j]) })
	sort.Slice(topology.orphans, func(i, j int) bool { return less(topology.orphans[i], topology.orphans[j]) })
	for parentID := range topology.children {
		sort.Slice(topology.children[parentID], func(i, j int) bool {
			return less(topology.children[parentID][i], topology.children[parentID][j])
		})
	}

	visited := make(map[string]bool, len(observation.spans))
	var walk func(string, int)
	walk = func(id string, depth int) {
		if visited[id] {
			return
		}
		visited[id] = true
		topology.depthByID[id] = depth
		topology.maximumDepth = max(topology.maximumDepth, depth)
		for _, childID := range topology.children[id] {
			walk(childID, depth+1)
		}
	}

	topology.displayRoots = append(topology.displayRoots, topology.roots...)
	topology.displayRoots = append(topology.displayRoots, topology.orphans...)
	// Production ranks genuine roots before missing-parent roots, then uses
	// start time within each class. Do not merge the classes by start time.
	for _, id := range topology.displayRoots {
		walk(id, 0)
	}
	for len(visited) < len(observation.spans) {
		unvisited := make([]string, 0)
		for id := range byID {
			if !visited[id] {
				unvisited = append(unvisited, id)
			}
		}
		sort.Slice(unvisited, func(i, j int) bool { return less(unvisited[i], unvisited[j]) })
		require.NotEmpty(t, unvisited)
		topology.displayRoots = append(topology.displayRoots, unvisited[0])
		walk(unvisited[0], 0)
	}
	return topology
}

func classifyTopology(topology fixtureTopology) string {
	if len(topology.cycleNodes) > 0 {
		return "cycle"
	}
	if len(topology.orphans) > 0 {
		return "orphan"
	}
	if len(topology.roots) > 1 {
		return "multiple-roots"
	}
	return "rooted-tree"
}

func assertSequentialSpanIDs(t *testing.T, observation fixtureObservation, count int) {
	t.Helper()
	seen := make([]bool, count+1)
	for _, span := range observation.spans {
		require.GreaterOrEqual(t, span.ordinal, uint64(1))
		require.LessOrEqual(t, span.ordinal, uint64(count))
		require.False(t, seen[span.ordinal], "span ordinal %d is duplicated", span.ordinal)
		seen[span.ordinal] = true
	}
	for ordinal := 1; ordinal <= count; ordinal++ {
		require.True(t, seen[ordinal], "span ordinal %d is missing", ordinal)
	}
}

func assertFixtureCoverage(t *testing.T, name string, observation fixtureObservation) {
	t.Helper()
	require.NotEmpty(t, observation.spans)
	require.Equal(t, observation.resourceCount, observation.resourceAttributeOwners)
	require.Equal(t, observation.scopeEnvelopeCount, observation.scopeAttributeOwners)
	require.Equal(t, len(observation.spans), observation.spanAttributeOwners)
	require.Equal(t, observation.eventCount, observation.eventAttributeOwners)
	require.Equal(t, observation.linkCount, observation.linkAttributeOwners)
	require.GreaterOrEqual(t, observation.eventCount, len(observation.spans))
	require.GreaterOrEqual(t, observation.linkCount, 2)
	require.Equal(t, 2, observation.maximumLinksOnSpan, "at least one span must carry multiple links")
	require.Len(t, observation.eventNames, observation.eventCount)
	require.Len(t, observation.eventTimestamps, observation.eventCount)
	require.Len(t, observation.linkTargets, observation.linkCount)
	require.Equal(t, len(observation.spans), observation.spanTraceStateCount)
	require.True(t, observation.spanFlagsPresent)
	require.True(t, observation.linkFlagsPresent)
	require.True(t, observation.resourceSchemaPresent)
	require.True(t, observation.scopeSchemaPresent)

	starts := make(map[uint64]struct{}, len(observation.spans))
	minimumStart := observation.spans[0].start
	for _, span := range observation.spans {
		minimumStart = min(minimumStart, span.start)
		_, duplicate := starts[span.start]
		require.False(t, duplicate, "span start %d is not globally unique", span.start)
		starts[span.start] = struct{}{}
	}
	const maxSafeInteger = uint64(1<<53 - 1)
	for _, span := range observation.spans {
		require.LessOrEqual(t, span.start-minimumStart, maxSafeInteger, "span start offset exceeds Number.MAX_SAFE_INTEGER")
		require.LessOrEqual(t, span.end-span.start, maxSafeInteger, "span duration exceeds Number.MAX_SAFE_INTEGER")
		for _, eventTime := range span.eventTimes {
			require.LessOrEqual(t, eventTime-minimumStart, maxSafeInteger, "event offset exceeds Number.MAX_SAFE_INTEGER")
		}
	}

	if name != "realistic-159" {
		return
	}
	require.Equal(t, 12, observation.resourceCount)
	require.Equal(t, 36, observation.scopeEnvelopeCount)
	require.Len(t, observation.services, 12)
	require.Len(t, observation.scopes, 3)
	require.Len(t, observation.kinds, 6, "headline fixture must contain every OTLP span kind")
	require.Contains(t, observation.statusCodes, ptrace.StatusCodeUnset)
	require.Contains(t, observation.statusCodes, ptrace.StatusCodeOk)
	require.Contains(t, observation.statusCodes, ptrace.StatusCodeError)
	require.Equal(t, 3, observation.errorCount)
	require.Equal(t, 181, observation.eventCount)
	require.Equal(t, 8, observation.linkCount)
	require.Equal(t, map[int]int{64: 53, 256: 53, 1024: 53}, observation.payloadSizes)
	require.True(t, observation.resourceDroppedAttributes)
	require.True(t, observation.scopeDroppedAttributes)
	require.True(t, observation.spanDroppedAttributes)
	require.True(t, observation.spanDroppedEvents)
	require.True(t, observation.spanDroppedLinks)
	require.True(t, observation.eventDroppedAttributes)
	require.True(t, observation.linkDroppedAttributes)
}

func assertUniqueSiblingAndDisplayRootStarts(t *testing.T, observation fixtureObservation, topology fixtureTopology) {
	t.Helper()
	byID := spansByID(observation)
	for parentID, children := range topology.children {
		starts := make(map[uint64]string, len(children))
		for _, childID := range children {
			start := byID[childID].start
			if previous, duplicate := starts[start]; duplicate {
				t.Fatalf("siblings %s and %s under parent %s share start %d", previous, childID, parentID, start)
			}
			starts[start] = childID
		}
	}
	displayRootStarts := make(map[uint64]string, len(topology.displayRoots))
	for _, rootID := range topology.displayRoots {
		start := byID[rootID].start
		if previous, duplicate := displayRootStarts[start]; duplicate {
			t.Fatalf("display roots %s and %s share start %d", previous, rootID, start)
		}
		displayRootStarts[start] = rootID
	}
}

func assertFixtureSpecificTopology(t *testing.T, name string, observation fixtureObservation, topology fixtureTopology) {
	t.Helper()
	byOrdinal := make(map[uint64]observedSpan, len(observation.spans))
	for _, span := range observation.spans {
		byOrdinal[span.ordinal] = span
	}

	switch name {
	case "realistic-159":
		widths := []int{1, 4, 8, 12, 16, 20, 22, 20, 16, 12, 10, 8, 5, 3, 2}
		require.Equal(t, widths, displayedLevelWidths(topology))
		previousLevelStart := uint64(0)
		previousLevelWidth := 0
		levelStart := uint64(1)
		for depth, width := range widths {
			for position := 0; position < width; position++ {
				ordinal := levelStart + uint64(position)
				span := byOrdinal[ordinal]
				require.Equal(t, depth, topology.depthByID[span.id])
				if depth == 0 {
					require.Empty(t, span.parentID)
					continue
				}
				expectedParent := previousLevelStart + uint64(position*previousLevelWidth/width)
				require.Equal(t, expectedParent, ordinalFromSpanID(t, span.parentID),
					"headline parent assignment changed for span %d", ordinal)
			}
			previousLevelStart = levelStart
			previousLevelWidth = width
			levelStart += uint64(width)
		}
		assertEnvelopeIsNotParentOrdered(t, observation)
	case "single-span":
		require.Equal(t, []int{1}, displayedLevelWidths(topology))
		require.Empty(t, byOrdinal[1].parentID)
	case "wide":
		require.Equal(t, []int{1, 24}, displayedLevelWidths(topology))
		for ordinal := uint64(2); ordinal <= 25; ordinal++ {
			require.Equal(t, uint64(1), ordinalFromSpanID(t, byOrdinal[ordinal].parentID))
		}
	case "deep":
		require.Equal(t, []int{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}, displayedLevelWidths(topology))
		for ordinal := uint64(2); ordinal <= 18; ordinal++ {
			require.Equal(t, ordinal-1, ordinalFromSpanID(t, byOrdinal[ordinal].parentID))
		}
	case "multiple-roots":
		require.Equal(t, []int{3, 3}, displayedLevelWidths(topology))
		require.Empty(t, byOrdinal[1].parentID)
		require.Empty(t, byOrdinal[3].parentID)
		require.Empty(t, byOrdinal[5].parentID)
		require.Equal(t, uint64(1), ordinalFromSpanID(t, byOrdinal[2].parentID))
		require.Equal(t, uint64(3), ordinalFromSpanID(t, byOrdinal[4].parentID))
		require.Equal(t, uint64(5), ordinalFromSpanID(t, byOrdinal[6].parentID))
	case "orphan":
		require.Equal(t, []int{2, 2}, displayedLevelWidths(topology))
		require.Equal(t, missingParentOrdinal, ordinalFromSpanID(t, byOrdinal[3].parentID))
		require.Equal(t, uint64(3), ordinalFromSpanID(t, byOrdinal[4].parentID))
		require.Less(t, byOrdinal[3].start, byOrdinal[1].start,
			"promoted orphan must start before the genuine root")
		require.Equal(t, byOrdinal[1].id, topology.displayRoots[0],
			"production displays genuine roots before earlier promoted orphans")
	case "cycle":
		require.Equal(t, []int{1, 1, 1}, displayedLevelWidths(topology))
		require.Equal(t, uint64(3), ordinalFromSpanID(t, byOrdinal[1].parentID))
		require.Equal(t, uint64(1), ordinalFromSpanID(t, byOrdinal[2].parentID))
		require.Equal(t, uint64(2), ordinalFromSpanID(t, byOrdinal[3].parentID))
		require.Len(t, topology.displayRoots, 1)
		require.Equal(t, byOrdinal[1].id, topology.displayRoots[0], "earliest cycle member must be the deterministic salvage root")
		for _, children := range topology.children {
			require.Len(t, children, 1, "cycle fixture must remain unbranched")
		}
	default:
		t.Fatalf("missing fixture-specific assertions for %q", name)
	}
}

func assertEnvelopeIsNotParentOrdered(t *testing.T, observation fixtureObservation) {
	t.Helper()
	positions := make(map[uint64]int, len(observation.envelopeOrdinals))
	for position, ordinal := range observation.envelopeOrdinals {
		positions[ordinal] = position
	}
	parentAppearsLater := false
	for _, span := range observation.spans {
		if span.parentID == "" {
			continue
		}
		parentOrdinal := ordinalFromSpanID(t, span.parentID)
		parentPosition, parentPresent := positions[parentOrdinal]
		if parentPresent && parentPosition > positions[span.ordinal] {
			parentAppearsLater = true
			break
		}
	}
	require.True(t, parentAppearsLater,
		"headline envelope accidentally became parent ordered; topology must not depend on input order")
}

func displayedLevelWidths(topology fixtureTopology) []int {
	widths := make([]int, topology.maximumDepth+1)
	for _, depth := range topology.depthByID {
		widths[depth]++
	}
	return widths
}

func spansByID(observation fixtureObservation) map[string]observedSpan {
	byID := make(map[string]observedSpan, len(observation.spans))
	for _, span := range observation.spans {
		byID[span.id] = span
	}
	return byID
}

func hasSpan(spans map[string]observedSpan, id string) bool {
	_, ok := spans[id]
	return ok
}

func decodeExportRequest(t *testing.T, data []byte) ptraceotlp.ExportRequest {
	t.Helper()
	request := ptraceotlp.NewExportRequest()
	require.NoError(t, request.UnmarshalProto(data))
	return request
}

func decodeManifest(t *testing.T, data []byte) fixtureManifest {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var manifest fixtureManifest
	require.NoError(t, decoder.Decode(&manifest))
	var trailing any
	require.ErrorIs(t, decoder.Decode(&trailing), io.EOF)
	return manifest
}

func ordinalFromSpanID(t *testing.T, id string) uint64 {
	t.Helper()
	decoded, err := hex.DecodeString(id)
	require.NoError(t, err)
	require.Len(t, decoded, 8)
	return binary.BigEndian.Uint64(decoded)
}

func spanIDHex(id pcommon.SpanID) string {
	return hex.EncodeToString(id[:])
}

func assertFiniteAttributeValue(t *testing.T, value any) {
	t.Helper()
	switch typed := value.(type) {
	case float64:
		require.False(t, math.IsNaN(typed), "fixture attributes must not contain NaN")
		require.False(t, math.IsInf(typed, 0), "fixture attributes must not contain infinity")
	case map[string]any:
		for _, nested := range typed {
			assertFiniteAttributeValue(t, nested)
		}
	case []any:
		for _, nested := range typed {
			assertFiniteAttributeValue(t, nested)
		}
	}
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
