//go:build waterfallbench

package main

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
)

var errFixtureNotFound = errors.New("waterfall benchmark fixture not found")

//go:embed testdata/manifest.json testdata/*.otlp.pb
var embeddedFixtureFiles embed.FS

func loadFixture(name string) (fixtureManifestEntry, []byte, error) {
	manifest, err := loadEmbeddedFixtureManifest()
	if err != nil {
		return fixtureManifestEntry{}, nil, err
	}
	for _, entry := range manifest.Fixtures {
		if entry.Name != name {
			continue
		}
		data, err := embeddedFixtureFiles.ReadFile(path.Join("testdata", entry.Filename))
		if err != nil {
			return fixtureManifestEntry{}, nil, fmt.Errorf("read embedded fixture %q: %w", name, err)
		}
		if len(data) != entry.Bytes {
			return fixtureManifestEntry{}, nil, fmt.Errorf(
				"embedded fixture %q has %d bytes, manifest records %d", name, len(data), entry.Bytes)
		}
		digest := sha256.Sum256(data)
		if hex.EncodeToString(digest[:]) != entry.SHA256 {
			return fixtureManifestEntry{}, nil, fmt.Errorf("embedded fixture %q does not match its manifest digest", name)
		}
		return entry, bytes.Clone(data), nil
	}
	return fixtureManifestEntry{}, nil, fmt.Errorf("%w: %q", errFixtureNotFound, name)
}

func loadEmbeddedFixtureManifest() (fixtureManifest, error) {
	data, err := embeddedFixtureFiles.ReadFile("testdata/manifest.json")
	if err != nil {
		return fixtureManifest{}, fmt.Errorf("read embedded fixture manifest: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var manifest fixtureManifest
	if err := decoder.Decode(&manifest); err != nil {
		return fixtureManifest{}, fmt.Errorf("decode embedded fixture manifest: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return fixtureManifest{}, fmt.Errorf("fixture manifest contains trailing JSON")
		}
		return fixtureManifest{}, fmt.Errorf("decode trailing fixture manifest data: %w", err)
	}
	if manifest.SchemaVersion != 1 {
		return fixtureManifest{}, fmt.Errorf("unsupported fixture manifest schema version %d", manifest.SchemaVersion)
	}
	if len(manifest.Fixtures) == 0 {
		return fixtureManifest{}, fmt.Errorf("fixture manifest is empty")
	}
	for i, entry := range manifest.Fixtures {
		if err := validateManifestEntry(entry); err != nil {
			return fixtureManifest{}, fmt.Errorf("validate fixture manifest entry %d: %w", i, err)
		}
		for _, previous := range manifest.Fixtures[:i] {
			if entry.Name == previous.Name {
				return fixtureManifest{}, fmt.Errorf("fixture manifest repeats name %q", entry.Name)
			}
			if entry.Filename == previous.Filename {
				return fixtureManifest{}, fmt.Errorf("fixture manifest repeats filename %q", entry.Filename)
			}
		}
	}
	return manifest, nil
}

func validateManifestEntry(entry fixtureManifestEntry) error {
	if entry.Name == "" {
		return fmt.Errorf("fixture name is empty")
	}
	if entry.Filename != entry.Name+".otlp.pb" {
		return fmt.Errorf("fixture %q has unexpected filename %q", entry.Name, entry.Filename)
	}
	if entry.Bytes <= 0 || entry.SpanCount <= 0 || entry.ExpectedDisplayedSpanCount <= 0 {
		return fmt.Errorf("fixture %q has non-positive byte or span metadata", entry.Name)
	}
	if entry.ExpectedDisplayedSpanCount > entry.SpanCount {
		return fmt.Errorf("fixture %q displays more spans than it contains", entry.Name)
	}
	if entry.ExpectedMaximumDisplayedDepth < 0 ||
		entry.ExpectedMaximumDisplayedDepth >= entry.ExpectedDisplayedSpanCount {
		return fmt.Errorf("fixture %q has invalid maximum displayed depth", entry.Name)
	}
	if err := validateHexID(entry.TraceID, 16); err != nil {
		return fmt.Errorf("fixture %q trace ID: %w", entry.Name, err)
	}
	if err := validateHexID(entry.ExpectedFirstSpanID, 8); err != nil {
		return fmt.Errorf("fixture %q expected first span ID: %w", entry.Name, err)
	}
	digest, err := hex.DecodeString(entry.SHA256)
	if err != nil || len(digest) != sha256.Size {
		return fmt.Errorf("fixture %q has invalid SHA-256 %q", entry.Name, entry.SHA256)
	}
	switch entry.Topology {
	case "rooted-tree", "multiple-roots", "orphan", "cycle":
		return nil
	default:
		return fmt.Errorf("fixture %q has unknown topology %q", entry.Name, entry.Topology)
	}
}

func validateHexID(value string, byteLength int) error {
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != byteLength {
		return fmt.Errorf("must be %d hexadecimal bytes", byteLength)
	}
	for _, b := range decoded {
		if b != 0 {
			return nil
		}
	}
	return fmt.Errorf("must be nonzero")
}
