package ingest_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.uber.org/zap"
)

func attrMap(kv map[string]string) pcommon.Map {
	m := pcommon.NewMap()
	for k, v := range kv {
		m.PutStr(k, v)
	}
	return m
}

// The whole model rests on this: the same content must produce the same id
// across batches and process restarts, with no database round trip.
func TestAttributeIDIsContentDerived(t *testing.T) {
	a := ingest.AttributeID("http.method", "GET", "string", ingest.ScopeSpan)
	b := ingest.AttributeID("http.method", "GET", "string", ingest.ScopeSpan)
	assert.Equal(t, a, b, "same content must give the same id")

	// Every field participates.
	assert.NotEqual(t, a, ingest.AttributeID("http.method", "POST", "string", ingest.ScopeSpan))
	assert.NotEqual(t, a, ingest.AttributeID("http.verb", "GET", "string", ingest.ScopeSpan))
	assert.NotEqual(t, a, ingest.AttributeID("http.method", "GET", "int64", ingest.ScopeSpan))
	assert.NotEqual(t, a, ingest.AttributeID("http.method", "GET", "string", ingest.ScopeResource),
		"scope is part of identity, so discovery can report attributeScope without unnesting owners")
}

// Length-prefixed framing: without it, ("ab","c") and ("a","bc") would hash the
// same, and a value containing the separator could impersonate a different
// split of the same bytes.
func TestHashFramingIsUnambiguous(t *testing.T) {
	assert.NotEqual(t,
		ingest.AttributeID("ab", "c", "string", ingest.ScopeSpan),
		ingest.AttributeID("a", "bc", "string", ingest.ScopeSpan))

	// A value full of plausible separators must not collide with anything.
	assert.NotEqual(t,
		ingest.AttributeID("k", "a|b=c,d", "string", ingest.ScopeSpan),
		ingest.AttributeID("k", "a|b=c", "string", ingest.ScopeSpan))
}

// Arrays are sorted by id and deduped, so two maps with the same content
// produce byte-identical arrays regardless of insertion order -- which is what
// makes resource dedupe work at all.
func TestAttributeSetIsOrderIndependent(t *testing.T) {
	one := pcommon.NewMap()
	one.PutStr("b", "2")
	one.PutStr("a", "1")
	one.PutStr("c", "3")

	two := pcommon.NewMap()
	two.PutStr("c", "3")
	two.PutStr("a", "1")
	two.PutStr("b", "2")

	_, idsOne := ingest.AttributeSet(one, ingest.ScopeResource)
	_, idsTwo := ingest.AttributeSet(two, ingest.ScopeResource)

	require.Equal(t, idsOne, idsTwo, "insertion order must not change the array")
	assert.Equal(t,
		ingest.ResourceID(idsOne, 0),
		ingest.ResourceID(idsTwo, 0),
		"so the same resource in a different order is still one resource")
}

// Dropped counts participate in resource identity, so a resource whose dropped
// count changes is a genuinely different row. Worth pinning: it means a flaky
// exporter that varies the count fragments the dedupe.
func TestResourceIdentityIncludesDroppedCount(t *testing.T) {
	_, ids := ingest.AttributeSet(attrMap(map[string]string{"service.name": "checkout"}), ingest.ScopeResource)
	assert.NotEqual(t, ingest.ResourceID(ids, 0), ingest.ResourceID(ids, 3))
}

// Two scopes with identical (empty) attributes are still different scopes.
func TestScopeIdentityIncludesNameAndVersion(t *testing.T) {
	var none []struct{}
	_ = none
	empty := pcommon.NewMap()
	_, ids := ingest.AttributeSet(empty, ingest.ScopeScope)

	assert.NotEqual(t,
		ingest.ScopeID("otelhttp", "1.0.0", ids, 0),
		ingest.ScopeID("otelsql", "1.0.0", ids, 0))
	assert.NotEqual(t,
		ingest.ScopeID("otelhttp", "1.0.0", ids, 0),
		ingest.ScopeID("otelhttp", "1.1.0", ids, 0))
}

func newStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.NewStore(context.Background(), "", zap.NewNop())
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

// Flush writes the dictionary, and flushing the same content twice must not
// duplicate anything -- that is the whole point of content-derived ids plus
// `on conflict do nothing`.
func TestFlushIsIdempotent(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	build := func() *ingest.Dictionary {
		d := ingest.NewDictionary(nil)
		res := pcommon.NewResource()
		res.Attributes().PutStr("service.name", "checkout")
		res.Attributes().PutStr("host.name", "pod-a")
		d.AddResource(res)

		sc := pcommon.NewInstrumentationScope()
		sc.SetName("otelhttp")
		sc.SetVersion("1.2.0")
		d.AddScope(sc)

		d.AddAttributes(attrMap(map[string]string{"http.method": "GET"}), ingest.ScopeSpan)
		return d
	}

	for i := 0; i < 3; i++ {
		require.NoError(t, s.WithConn(func(conn driver.Conn) error {
			return build().Flush(ctx, conn)
		}), "flush %d", i)
	}

	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		var attrs, resources, scopes int
		require.NoError(t, db.QueryRow(`select count(*) from attributes`).Scan(&attrs))
		require.NoError(t, db.QueryRow(`select count(*) from resources`).Scan(&resources))
		require.NoError(t, db.QueryRow(`select count(*) from scopes`).Scan(&scopes))

		// 2 resource attributes + 1 span attribute; the scope has none.
		assert.Equal(t, 3, attrs, "three flushes of the same content must write it once")
		assert.Equal(t, 1, resources)
		assert.Equal(t, 1, scopes)
		return nil
	}))
}

// A failed exec must never mark ids as flushed. Marking before -- or
// regardless of -- a successful exec would tell the cache a row exists that
// was never written, which is the one failure mode FlushedIDs' doc comment
// calls out as undetectable: a later batch would trust the cache, skip the
// insert, and leave an owner's array pointing at a row that was never there.
func TestFailedFlushDoesNotMarkAsFlushed(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	// Force the attributes insert to fail without touching FlushedIDs itself:
	// drop the table the statement targets.
	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
		_, err := db.Exec(`drop table attributes`)
		return err
	}))

	d := ingest.NewDictionary(s.FlushedIDs())
	d.AddAttributes(attrMap(map[string]string{"http.method": "GET"}), ingest.ScopeSpan)

	err := s.WithConn(func(conn driver.Conn) error {
		return d.Flush(ctx, conn)
	})
	require.Error(t, err, "the insert must fail with its table gone")
	assert.Zero(t, s.FlushedIDs().Len(),
		"a failed exec must leave nothing marked -- marking beforehand would convince the cache this row exists")
}

// Every id an owner array references must exist in the dictionary. Nothing
// enforces this -- DuckDB cannot FK into a LIST -- so it is checked rather than
// assumed.
func TestFlushedArraysReferenceRealAttributes(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	d := ingest.NewDictionary(nil)
	res := pcommon.NewResource()
	res.Attributes().PutStr("service.name", "checkout")
	res.Attributes().PutInt("process.pid", 4242)
	d.AddResource(res)

	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return d.Flush(ctx, conn)
	}))

	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		var dangling int
		err := db.QueryRow(`
			select count(*) from (
				select unnest(attribute_ids) as id from resources
			) r
			where not exists (select 1 from attributes a where a.id = r.id)
		`).Scan(&dangling)
		require.NoError(t, err)
		assert.Zero(t, dangling, "resource arrays must not reference missing dictionary rows")
		return nil
	}))
}

// The stored ids must match what the SQL macro recomputes. The macro is an
// independent reimplementation, so this catches a bug in the Go hashing itself
// rather than only storage corruption.
func TestStoredIDsMatchTheSQLMacro(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	d := ingest.NewDictionary(nil)
	d.AddAttributes(attrMap(map[string]string{
		"http.method": "GET",
		"url.path":    "/api/v1/resource",
		"café":        "unicode value",
	}), ingest.ScopeSpan)

	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return d.Flush(ctx, conn)
	}))

	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		var mismatched int
		err := db.QueryRow(
			`select count(*) from attributes where id <> attr_id(key, value, type::varchar, scope)`,
		).Scan(&mismatched)
		require.NoError(t, err)
		assert.Zero(t, mismatched, "Go and SQL must agree on every id")
		return nil
	}))
}
