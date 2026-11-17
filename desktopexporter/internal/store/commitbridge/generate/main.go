// Command generate copies the commit bridge's headers from pinned DuckDB source.
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

const (
	source        = "d54cbbe3df17e8e5ce68312f483b4155836112aa"
	archiveSHA256 = "93e3d1f9c84b6ec3493237d21565c01c2c19b9862e54e5552d3d7999ccc3bbb4"
	bridgeDir     = "desktopexporter/internal/store/commitbridge"
)

var (
	quotedInclude = regexp.MustCompile(`#include "([^"]+)"`)
	include       = regexp.MustCompile(`#include ["<]([^">]+)[">]`)
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./"+bridgeDir+"/generate <duckdb-source.tar.gz> (from the repository root)")
		os.Exit(1)
	}
	if err := generate(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func generate(archivePath string) error {
	data, err := os.ReadFile(archivePath)
	if err != nil {
		return err
	}
	if fmt.Sprintf("%x", sha256.Sum256(data)) != archiveSHA256 {
		return fmt.Errorf("DuckDB source archive checksum does not match the pinned archive")
	}
	headers, licence, err := readArchive(data)
	if err != nil {
		return err
	}
	bridge, err := os.ReadFile(filepath.Join(bridgeDir, "bridge.cpp"))
	if err != nil {
		return err
	}

	queue := quotedInclude.FindAllSubmatch(bridge, -1)
	selected := make(map[string][]byte)
	for len(queue) > 0 {
		name := string(queue[len(queue)-1][1])
		queue = queue[:len(queue)-1]
		if _, ok := selected[name]; ok {
			continue
		}
		header, ok := headers[name]
		if !ok {
			return fmt.Errorf("DuckDB source archive is missing header %q", name)
		}
		if !filepath.IsLocal(name) {
			return fmt.Errorf("DuckDB header path is not relative to include/: %q", name)
		}
		selected[name] = header
		for _, dependency := range include.FindAllSubmatch(header, -1) {
			if _, ok := headers[string(dependency[1])]; ok {
				queue = append(queue, dependency)
			}
		}
	}

	names := make([]string, 0, len(selected))
	for name := range selected {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		target := filepath.Join(bridgeDir, "include", filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, selected[name], 0o644); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(bridgeDir, "LICENSE.duckdb"), licence, 0o644); err != nil {
		return err
	}
	fmt.Printf("Copied %d DuckDB headers from %s\n", len(selected), source)
	return nil
}

func readArchive(data []byte) (map[string][]byte, []byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, nil, err
	}
	defer gz.Close()

	prefix := "duckdb-" + source + "/"
	headers := make(map[string][]byte)
	var licence []byte
	archive := tar.NewReader(gz)
	for {
		entry, err := archive.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, err
		}
		if !entry.FileInfo().Mode().IsRegular() {
			continue
		}
		isHeader := strings.HasPrefix(entry.Name, prefix+"src/include/")
		if !isHeader && entry.Name != prefix+"LICENSE" {
			continue
		}
		contents, err := io.ReadAll(archive)
		if err != nil {
			return nil, nil, err
		}
		if isHeader {
			headers[strings.TrimPrefix(entry.Name, prefix+"src/include/")] = contents
		} else {
			licence = contents
		}
	}
	if licence == nil {
		return nil, nil, fmt.Errorf("DuckDB source archive is missing LICENSE")
	}
	return headers, licence, nil
}
