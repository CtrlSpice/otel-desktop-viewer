// Package evals archives original JSON results. It does not interpret formats,
// execute evaluators, or resolve references contained in a snapshot.
package evals

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MaxPayloadBytes = 16 << 20
	MaxSourceBytes  = 1024
	MaxNameBytes    = 256
	DefaultLimit    = 50
	MaxLimit        = 100
	MaxOffset       = 1_000_000
)

var (
	ErrInvalidSnapshot  = errors.New("invalid eval snapshot")
	ErrSnapshotNotFound = errors.New("eval snapshot not found")
)

type Metadata struct {
	ID         string    `json:"id"`
	Digest     string    `json:"digest"`
	Source     string    `json:"source"`
	Format     string    `json:"format"`
	Name       string    `json:"name"`
	ImportedAt time.Time `json:"importedAt"`
	ByteSize   int64     `json:"byteSize"`
}

type Snapshot struct {
	Metadata
	OriginalJSON string `json:"originalJSON"`
}

type ImportResult struct {
	Snapshot Metadata `json:"snapshot"`
	Created  bool     `json:"created"`
}

type Page struct {
	Snapshots []Metadata `json:"snapshots"`
	HasMore   bool       `json:"hasMore"`
}

type DeleteResult struct {
	ID      string `json:"id"`
	Deleted bool   `json:"deleted"`
}

func ValidateID(id string) error {
	if len(id) != 64 || strings.Trim(id, "0123456789abcdef") != "" {
		return fmt.Errorf("%w: id must be 64 lowercase hexadecimal characters", ErrInvalidSnapshot)
	}
	return nil
}

func validateImport(source, format, name, original string) error {
	for _, field := range []struct {
		name, value string
		max         int
	}{
		{"source", source, MaxSourceBytes}, {"name", name, MaxNameBytes},
	} {
		if !utf8.ValidString(field.value) || len(field.value) == 0 || len(field.value) > field.max || strings.ContainsRune(field.value, 0) {
			return fmt.Errorf("%w: %s must be UTF-8 text, 1..%d bytes, without NUL", ErrInvalidSnapshot, field.name, field.max)
		}
	}
	if format != "promptfoo-v3" && format != "viewer-kit" && format != "unknown-json" {
		return fmt.Errorf("%w: unsupported format label", ErrInvalidSnapshot)
	}
	if len(original) == 0 || len(original) > MaxPayloadBytes || !utf8.ValidString(original) || !json.Valid([]byte(original)) {
		return fmt.Errorf("%w: originalJSON must be valid UTF-8 JSON, at most %d bytes", ErrInvalidSnapshot, MaxPayloadBytes)
	}
	return nil
}

const metadataColumns = `id, source, format, name, imported_at, byte_size`

type scanner interface{ Scan(...any) error }

func scanMetadata(row scanner, m *Metadata, extra ...any) error {
	fields := []any{&m.ID, &m.Source, &m.Format, &m.Name, &m.ImportedAt, &m.ByteSize}
	err := row.Scan(append(fields, extra...)...)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrSnapshotNotFound
	}
	m.Digest = m.ID
	return err
}

// Import must run inside Store.WithDBWrite. The write boundary serializes the
// existence check and insert with other imports/deletes. The primary key is an
// additional uniqueness guard. An exact repeat never updates first-import metadata.
func Import(ctx context.Context, db *sql.DB, source, format, name, original string) (ImportResult, error) {
	var out ImportResult
	if err := ctx.Err(); err != nil {
		return out, err
	}
	if err := validateImport(source, format, name, original); err != nil {
		return out, err
	}
	digest := sha256.Sum256([]byte(original))
	id := hex.EncodeToString(digest[:])
	err := scanMetadata(db.QueryRowContext(ctx, `select `+metadataColumns+` from eval_snapshots where id = ?`, id), &out.Snapshot)
	if err == nil {
		return out, nil
	}
	if !errors.Is(err, ErrSnapshotNotFound) {
		return out, err
	}
	// Go accepts lone surrogate escapes that DuckDB's JSON type rejects. Surface
	// that as invalid input rather than an internal database failure.
	var valid bool
	if err := db.QueryRowContext(ctx, `select json_valid(?)`, original).Scan(&valid); err != nil {
		return out, err
	}
	if !valid {
		return out, fmt.Errorf("%w: originalJSON is not valid DuckDB JSON (check Unicode surrogate escapes)", ErrInvalidSnapshot)
	}
	err = scanMetadata(db.QueryRowContext(ctx, `insert into eval_snapshots (id, source, format, name, byte_size, payload)
		values (?, ?, ?, ?, ?, cast(? as json)) returning `+metadataColumns,
		id, source, format, name, len(original), original), &out.Snapshot)
	out.Created = err == nil
	return out, err
}

// Search reads only metadata, with one look-ahead row. Call inside WithDBRead.
func Search(ctx context.Context, db *sql.DB, limit, offset int64) (Page, error) {
	out := Page{Snapshots: []Metadata{}}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	if limit < 1 || limit > MaxLimit || offset < 0 || offset > MaxOffset {
		return out, fmt.Errorf("%w: limit must be 1..%d and offset 0..%d", ErrInvalidSnapshot, MaxLimit, MaxOffset)
	}
	rows, err := db.QueryContext(ctx, `select `+metadataColumns+` from eval_snapshots order by imported_at desc, id asc limit ? offset ?`, limit+1, offset)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var m Metadata
		if err := scanMetadata(rows, &m); err != nil {
			return out, err
		}
		if int64(len(out.Snapshots)) == limit {
			out.HasMore = true
			break
		}
		out.Snapshots = append(out.Snapshots, m)
	}
	return out, rows.Err()
}

// Get casts native JSON directly to text, without JSON reconstruction. Call
// inside WithDBRead. Never scan the JSON column into a parsed Go value.
func Get(ctx context.Context, db *sql.DB, id string) (Snapshot, error) {
	var out Snapshot
	if err := ctx.Err(); err != nil {
		return out, err
	}
	if err := ValidateID(id); err != nil {
		return out, err
	}
	err := scanMetadata(db.QueryRowContext(ctx, `select `+metadataColumns+`, cast(payload as varchar) from eval_snapshots where id = ?`, id), &out.Metadata, &out.OriginalJSON)
	return out, err
}

// Delete is the only archive deletion operation. Call inside WithDBWrite.
func Delete(ctx context.Context, db *sql.DB, id string) (DeleteResult, error) {
	out := DeleteResult{ID: id}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	if err := ValidateID(id); err != nil {
		return out, err
	}
	var deleted string
	err := db.QueryRowContext(ctx, `delete from eval_snapshots where id = ? returning id`, id).Scan(&deleted)
	if errors.Is(err, sql.ErrNoRows) {
		return out, ErrSnapshotNotFound
	}
	out.Deleted = err == nil
	return out, err
}
