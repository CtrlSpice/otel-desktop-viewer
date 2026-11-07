package ingest

import (
	"crypto/sha256"
	"strconv"
	"strings"

	"github.com/duckdb/duckdb-go/v2"
	"github.com/google/uuid"
)

// Content identities use length-prefixed parts and the first 128 SHA-256 bits.
// They identify received payloads; they do not reference attribute tables.
func hashID(parts ...string) duckdb.UUID {
	h := sha256.New()
	for _, part := range parts {
		h.Write([]byte(strconv.Itoa(len(part))))
		h.Write([]byte(":"))
		h.Write([]byte(part))
	}
	var id duckdb.UUID
	copy(id[:], h.Sum(nil)[:16])
	return id
}

func AttributeID(key, value string) duckdb.UUID { return hashID(key, value) }

func ResourcePayloadID(attributes []duckdb.UUID, dropped uint32) duckdb.UUID {
	return hashID(uuidsKey(attributes), strconv.FormatUint(uint64(dropped), 10))
}

func ResourceID(payloadID duckdb.UUID, schemaURL string) duckdb.UUID {
	return hashID(uuidString(payloadID), schemaURL)
}

func ScopeID(name, version, schemaURL string, attributes []duckdb.UUID, dropped uint32) duckdb.UUID {
	return hashID(name, version, schemaURL, uuidsKey(attributes), strconv.FormatUint(uint64(dropped), 10))
}

func uuidsKey(ids []duckdb.UUID) string {
	var b strings.Builder
	b.Grow(len(ids) * 33)
	for _, id := range ids {
		b.WriteString(uuidString(id))
		b.WriteByte(',')
	}
	return b.String()
}

func uuidString(id duckdb.UUID) string {
	const digits = "0123456789abcdef"
	var out [32]byte
	for i, b := range id {
		out[i*2], out[i*2+1] = digits[b>>4], digits[b&15]
	}
	return string(out[:])
}

func uuidLess(a, b duckdb.UUID) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

func FormatUUID(id duckdb.UUID) string {
	h := uuidString(id)
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

func parseUUID(s string) (duckdb.UUID, error) {
	id, err := uuid.Parse(s)
	return duckdb.UUID(id), err
}
