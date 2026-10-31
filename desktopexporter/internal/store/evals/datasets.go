package evals

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	MaxNameBytes = 256
	MaxOffset    = 1_000_000
)

var (
	ErrInvalidEval  = errors.New("invalid eval data")
	ErrEvalNotFound = errors.New("eval data not found")
)

type Metadata struct {
	ID         string
	Source     string
	Format     string
	Name       string
	ImportedAt time.Time
	ByteSize   int64
}

const metadataColumns = `id, source, format, name, imported_at, byte_size`

type scanner interface{ Scan(...any) error }

func scanMetadata(row scanner, metadata *Metadata, extra ...any) error {
	fields := []any{&metadata.ID, &metadata.Source, &metadata.Format, &metadata.Name, &metadata.ImportedAt, &metadata.ByteSize}
	err := row.Scan(append(fields, extra...)...)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrEvalNotFound
	}
	return err
}

func ValidateID(id string) error {
	if len(id) != 64 || strings.Trim(id, "0123456789abcdef") != "" {
		return fmt.Errorf("%w: id must be 64 lowercase hexadecimal characters", ErrInvalidEval)
	}
	return nil
}

func DatasetID(importID string, envelope int) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s:%d", importID, envelope))))
}

// AppendParts writes a bounded cache batch. Production callers supply only
// reconstructed retained evidence, never upload/staging contents.
func AppendParts(ctx context.Context, db *sql.DB, importID, name string, parts []IngestPart) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, part := range parts {
		payload := nativeJSON(part.Payload)
		if part.Row >= 0 {
			_, err = tx.ExecContext(ctx, `insert into eval_records
			 values ($1, $2, $3, greatest($4, octet_length(encode(to_json($5::json::variant)::varchar))), $5::json::variant)`, importID, part.Envelope, part.Row, len(part.Payload), payload)
		} else {
			_, err = tx.ExecContext(ctx, `insert into eval_datasets (id, import_id, envelope, source, format, name, byte_size, row_count, context) values (?, ?, ?, 'json-import', ?, ?, ?, ?, ?::json::variant) on conflict(id) do update set format=excluded.format, byte_size=excluded.byte_size, row_count=excluded.row_count, context=excluded.context`, DatasetID(importID, part.Envelope), importID, part.Envelope, part.Format, name, part.Bytes, part.Count, payload)
		}
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Native JSON inference treats bare -0 as an integer. Give it a fractional
// part so VARIANT stores a signed DOUBLE. Numbers outside native range remain
// text instead of becoming rounded integers or infinities in the inspection UI.
func nativeJSON(raw []byte) string {
	var out bytes.Buffer
	for i := 0; i < len(raw); {
		start := i
		if raw[i] == '"' {
			i++
			for i < len(raw) {
				c := raw[i]
				i++
				if c == '\\' && i < len(raw) {
					i++
				} else if c == '"' {
					break
				}
			}
		} else if raw[i] == '-' || raw[i] >= '0' && raw[i] <= '9' {
			for i < len(raw) && strings.ContainsRune("-+0123456789.eE", rune(raw[i])) {
				i++
			}
			number := string(raw[start:i])
			if number == "-0" {
				out.WriteString("-0.0")
				continue
			}
			var err error
			if strings.ContainsAny(number, ".eE") {
				_, err = strconv.ParseFloat(number, 64)
			} else if strings.HasPrefix(number, "-") {
				_, err = strconv.ParseInt(number, 10, 64)
			} else {
				_, err = strconv.ParseUint(number, 10, 64)
			}
			if err != nil {
				out.WriteByte('"')
				out.WriteString(number)
				out.WriteByte('"')
				continue
			}
		} else {
			i++
		}
		out.Write(raw[start:i])
	}
	return out.String()
}

// Publish is the file's visibility boundary. Byte sizes describe serialized data.
func Publish(ctx context.Context, db *sql.DB, id string) error {
	_, err := db.ExecContext(ctx, `update eval_imports set complete = true where id = ?`, id)
	return err
}

type RecordPage struct {
	ID      string            `json:"id"`
	Format  string            `json:"format"`
	Context json.RawMessage   `json:"context"`
	Rows    []json.RawMessage `json:"rows"`
	Offset  int64             `json:"offset"`
	Total   int64             `json:"total"`
	HasMore bool              `json:"hasMore"`
}

// Records returns an ordered byte-bounded page plus the original experiment context.
// Clients must preserve row ordinals when constructing source pointers.
func Records(ctx context.Context, db *sql.DB, id string, offset, limit int64) (RecordPage, error) {
	out := RecordPage{ID: id, Offset: offset, Rows: []json.RawMessage{}}
	if err := ValidateID(id); err != nil {
		return out, err
	}
	if offset < 0 || limit < 1 || limit > 100 {
		return out, fmt.Errorf("%w: offset must be nonnegative and limit 1..100", ErrInvalidEval)
	}
	var importID, raw string
	var envelope int
	err := db.QueryRowContext(ctx, `select d.import_id, d.envelope, d.format, to_json(d.context)::varchar, d.row_count from eval_datasets d join eval_imports i on i.id = d.import_id where d.id = ? and i.complete and not d.deleted`, id).Scan(&importID, &envelope, &out.Format, &raw, &out.Total)
	if err == sql.ErrNoRows {
		return out, ErrEvalNotFound
	}
	if err != nil {
		return out, err
	}
	out.Context = json.RawMessage(raw)
	// Select the byte-bounded ordinal range before asking DuckDB/the driver to
	// materialize payloads; LIMIT 100 alone could materialize 400 MiB.
	sizes, err := db.QueryContext(ctx, `select byte_size from eval_records where import_id = ? and envelope = ? and ordinal >= ? order by ordinal limit ?`, importID, envelope, offset, limit)
	if err != nil {
		return out, err
	}
	end, bytes := offset, int64(0)
	for sizes.Next() {
		var size int64
		if err := sizes.Scan(&size); err != nil {
			sizes.Close()
			return out, err
		}
		if bytes+size > MaxRecordBytes && end > offset {
			break
		}
		bytes += size
		end++
	}
	err = sizes.Err()
	sizes.Close()
	if err != nil {
		return out, err
	}
	rows, err := db.QueryContext(ctx, `select to_json(payload)::varchar from eval_records where import_id = ? and envelope = ? and ordinal >= ? and ordinal < ? order by ordinal`, importID, envelope, offset, end)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return out, err
		}
		out.Rows = append(out.Rows, json.RawMessage(raw))
	}
	out.HasMore = offset+int64(len(out.Rows)) < out.Total
	return out, rows.Err()
}
