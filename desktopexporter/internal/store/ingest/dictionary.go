package ingest

import (
	"context"
	"crypto/sha256"
	"database/sql/driver"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/util"
	"github.com/duckdb/duckdb-go/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

// Attribute scopes. These are part of dictionary identity, not free-form tags:
// the same (key, value, type) under two scopes is two rows, which is what lets
// attribute discovery report attributeScope from a single dictionary scan
// instead of unnesting every owner array.
const (
	ScopeResource  = "resource"
	ScopeScope     = "scope"
	ScopeSpan      = "span"
	ScopeEvent     = "event"
	ScopeLink      = "link"
	ScopeLog       = "log"
	ScopeDatapoint = "datapoint"
	ScopeExemplar  = "exemplar"
)

// Attribute is one dictionary row: a distinct (key, value, type, scope).
type Attribute struct {
	ID    duckdb.UUID
	Key   string
	Value string
	Type  string
	Scope string
}

// Resource is one deduped resource: an attribute set plus its dropped count.
type Resource struct {
	ID           duckdb.UUID
	AttributeIDs []duckdb.UUID
	Dropped      uint32
}

// Scope is one deduped instrumentation scope.
type Scope struct {
	ID           duckdb.UUID
	Name         string
	Version      string
	AttributeIDs []duckdb.UUID
	Dropped      uint32
}

// hashID derives a 16-byte id from length-prefixed parts.
//
// Length prefixes rather than a separator: joining on a delimiter lets a value
// containing that delimiter collide with a different split of the same bytes.
// Prefixing each part with its byte length makes the encoding unambiguous, so
// there is no separator to escape and no encoding decision left to revisit.
//
// sha256 truncated to 16 bytes, to fit a UUID. Width is the only real lever on
// collisions and 128 bits is far past where it matters: the birthday bound is
// ~1.5e-27 at a million distinct entries, against fleet studies finding ~1% of
// DIMMs see an uncorrectable error per year. The machine underneath is orders
// of magnitude likelier to corrupt this than the hash is.
//
// Not salted, deliberately: the id must be a pure function of content or the
// same attribute arriving in two batches would get two ids and nothing would
// dedupe.
func hashID(parts ...string) duckdb.UUID {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(strconv.Itoa(len(p))))
		h.Write([]byte(":"))
		h.Write([]byte(p))
	}
	var id duckdb.UUID
	copy(id[:], h.Sum(nil)[:16])
	return id
}

// AttributeID returns the dictionary id for one attribute.
//
// Mirrored by the attr_id SQL macro, which exists as an independent
// reimplementation for the integrity check -- a Go-side re-hash would use this
// same function and could only catch storage corruption, not a bug in here.
func AttributeID(key, value, typ, scope string) duckdb.UUID {
	return hashID(key, value, typ, scope)
}

// ResourceID derives a resource's identity from its attribute ids and dropped
// count. The ids are already sorted and deduped by the caller.
func ResourceID(attributeIDs []duckdb.UUID, dropped uint32) duckdb.UUID {
	return hashID(uuidsKey(attributeIDs), strconv.FormatUint(uint64(dropped), 10))
}

// SeriesID derives a timeseries' identity: the stream it belongs to, which
// instance emitted it, and its label set.
//
// Content-derived like the rest, which is what makes it usable in a URL --
// the same series gets the same id across restarts and re-ingests, so a link
// survives in a way one built on a minted datapoint id cannot.
//
// The instance key is in there because metric_streams identifies a stream by
// service_name so a counter survives a pod restart, which means two replicas
// share a stream. The series is where they separate.
//
// It is deliberately NOT the resource row id, which this used to take. A
// resource id is a hash of the resource's whole attribute set, so anything
// enriching a resource mid-stream -- a processor that starts resolving k8s
// metadata, an SDK that adds telemetry.sdk.* partway through -- mints a new
// resource, and used to mint a new series with it. One instrument from one
// instance then drew two chart lines, split at the moment those attributes
// appeared, and a ?series= link addressed only one half of it. Measured in the
// reference capture: 48 resource rows for 35 distinct service.instance.id,
// with telemetry.sdk.* on 35 of them and absent from the other 13.
//
// See InstanceKey for the identity used instead.
func SeriesID(streamID duckdb.UUID, instanceKey string, attributeIDs []duckdb.UUID) duckdb.UUID {
	return hashID(formatUUID(streamID), instanceKey, uuidsKey(attributeIDs))
}

// instanceKeys are the resource attributes that identify *which instance* a
// resource describes, rather than merely describing it, most specific first.
//
// service.instance.id is the one OTel actually sanctions for this. host.name
// and k8s.pod.name are the usual stand-ins when an SDK omits it, and are what
// the original split-by-resource was reaching for.
var instanceKeys = []string{
	"service.instance.id",
	"host.name",
	"k8s.pod.name",
}

// InstanceKey returns a stable identity for the instance a resource describes,
// for use as part of a series id. Empty when the resource does not say.
//
// The attribute name is included in the result, so a host.name of "web-1"
// cannot collide with a k8s.pod.name of "web-1".
//
// Absent means absent. When none of these attributes are present the key is
// empty and every such resource lands on one series -- the same default a
// datapoint with no labels gets, for the same reason: the telemetry expressed
// no distinction, so we do not invent one.
//
// Falling back to the resource id here would be inventing one. A resource id is
// a hash of the whole attribute set, so it splits on enrichment rather than on
// instance, which means it answers a question nobody asked and answers it
// wrongly -- and it would do so precisely in the cases where we know least
// about the instance. Two replicas sharing a line because neither identified
// itself is a true statement about the data. Two lines because a processor
// added an attribute is not.
func InstanceKey(attrs pcommon.Map) string {
	for _, k := range instanceKeys {
		if v, ok := attrs.Get(k); ok {
			if s := v.AsString(); s != "" {
				return k + "=" + s
			}
		}
	}
	return ""
}

// ScopeID derives a scope's identity. Name and version participate because two
// instrumentation libraries with identical (empty) attribute sets are still
// different scopes.
func ScopeID(name, version string, attributeIDs []duckdb.UUID, dropped uint32) duckdb.UUID {
	return hashID(name, version, uuidsKey(attributeIDs), strconv.FormatUint(uint64(dropped), 10))
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
	const hexdigits = "0123456789abcdef"
	var out [32]byte
	for i, b := range id {
		out[i*2] = hexdigits[b>>4]
		out[i*2+1] = hexdigits[b&0x0f]
	}
	return string(out[:])
}

// AttributeSet turns an attribute map into dictionary rows plus the sorted,
// deduped id array an owner stores.
//
// Sorted by id, not by key. Ids are unique per distinct entry, so sorting on
// them is a total order needing no tie-break -- whereas sorting by key is only
// a total order if keys are unique, which pcommon.Map's mutation API enforces
// but the type itself does not check. Nothing is lost: array order never
// reaches the wire, because the read path orders by key in SQL.
//
// Deduping is a guard that should never fire; if it ever did, [A,A,B] and [A,B]
// would hash to different resources.
func AttributeSet(attrs pcommon.Map, scope string) ([]Attribute, []duckdb.UUID) {
	if attrs.Len() == 0 {
		return nil, nil
	}

	rows := make([]Attribute, 0, attrs.Len())
	for k, v := range attrs.All() {
		value, typ := util.ValueToStringAndType(v)
		rows = append(rows, Attribute{
			ID:    AttributeID(k, value, typ, scope),
			Key:   k,
			Value: value,
			Type:  typ,
			Scope: scope,
		})
	}

	sort.Slice(rows, func(i, j int) bool {
		return uuidLess(rows[i].ID, rows[j].ID)
	})

	ids := make([]duckdb.UUID, 0, len(rows))
	unique := rows[:0]
	for i, r := range rows {
		if i > 0 && r.ID == rows[i-1].ID {
			continue
		}
		unique = append(unique, r)
		ids = append(ids, r.ID)
	}
	return unique, ids
}

func uuidLess(a, b duckdb.UUID) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

// Dictionary accumulates the distinct attributes, resources and scopes seen
// while walking one OTLP request, then writes them in three statements.
//
// Deduping in Go first is what keeps the write bounded by *distinct* entries
// rather than by signal count: the reference capture's 723,692 attribute rows
// are 267 distinct entries.
type Dictionary struct {
	attributes map[duckdb.UUID]Attribute
	resources  map[duckdb.UUID]Resource
	scopes     map[duckdb.UUID]Scope

	// flushed is the store's record of what is already in the database. May be
	// nil, which disables the optimisation and restores the always-insert
	// behaviour -- see FlushedIDs.
	flushed *FlushedIDs
}

// NewDictionary builds a dictionary for one batch.
//
// flushed is the owning store's FlushedIDs, or nil to insert unconditionally.
// It must belong to the same store as the connection the batch is flushed on:
// a set shared across two databases would skip inserts into one of them.
func NewDictionary(flushed *FlushedIDs) *Dictionary {
	return &Dictionary{
		attributes: map[duckdb.UUID]Attribute{},
		resources:  map[duckdb.UUID]Resource{},
		scopes:     map[duckdb.UUID]Scope{},
		flushed:    flushed,
	}
}

// AddAttributes records an attribute map under a scope and returns the id array
// its owner should store.
func (d *Dictionary) AddAttributes(attrs pcommon.Map, scope string) []duckdb.UUID {
	rows, ids := AttributeSet(attrs, scope)
	for _, r := range rows {
		d.attributes[r.ID] = r
	}
	return ids
}

// AddResource records a resource and returns its id.
func (d *Dictionary) AddResource(res pcommon.Resource) duckdb.UUID {
	ids := d.AddAttributes(res.Attributes(), ScopeResource)
	id := ResourceID(ids, res.DroppedAttributesCount())
	d.resources[id] = Resource{ID: id, AttributeIDs: ids, Dropped: res.DroppedAttributesCount()}
	return id
}

// AddScope records an instrumentation scope and returns its id.
func (d *Dictionary) AddScope(scope pcommon.InstrumentationScope) duckdb.UUID {
	ids := d.AddAttributes(scope.Attributes(), ScopeScope)
	id := ScopeID(scope.Name(), scope.Version(), ids, scope.DroppedAttributesCount())
	d.scopes[id] = Scope{
		ID:           id,
		Name:         scope.Name(),
		Version:      scope.Version(),
		AttributeIDs: ids,
		Dropped:      scope.DroppedAttributesCount(),
	}
	return id
}

// Counts reports what this batch contributed, for ingest instrumentation.
func (d *Dictionary) Counts() (attributes, resources, scopes int) {
	return len(d.attributes), len(d.resources), len(d.scopes)
}

// Flush writes the dictionary.
//
// `on conflict do nothing` rather than `do update`: DO UPDATE fires for every
// conflicting row and a DuckDB update is a delete plus an insert, so on a
// dictionary that is mostly hits it would trade one read for N writes.
//
// Attributes go first. Nothing enforces the ordering -- DuckDB cannot FK into a
// LIST -- so a crash between statements leaves orphan dictionary rows, which
// the sweep collects, rather than resources referencing rows that do not exist.
//
// These are SQL inserts rather than appender writes because the appender has no
// conflict handling at all: its entire surface is AppendRow/Flush/Clear/Close,
// and a duplicate would error at flush and take the whole chunk with it.
func (d *Dictionary) Flush(ctx context.Context, conn driver.Conn) error {
	if err := d.flushAttributes(ctx, conn); err != nil {
		return err
	}
	if err := d.flushResources(ctx, conn); err != nil {
		return err
	}
	return d.flushScopes(ctx, conn)
}

// flushRows is the sequence every dictionary flush shares: filter down to
// rows the cache has not seen, bail out if nothing is left, execute, and mark
// only once that execution has actually succeeded.
//
// That ordering is the one thing that matters here and it used to be retyped
// three times, once per table. Marking before a successful exec -- or
// unconditionally -- would tell the cache a row exists that was never
// written, which is exactly the undetectable failure FlushedIDs documents:
// nothing catches it until an owner's array points at a row that silently
// never made it in.
//
// query must be `unnest`-shaped over the parallel arrays buildArgs returns,
// the same idiom metrics.go uses for the metric_streams upsert: one bound
// argument per column, so the statement text is fixed regardless of how many
// rows are in m.
func flushRows[T any](
	ctx context.Context,
	conn driver.Conn,
	flushed *FlushedIDs,
	m map[duckdb.UUID]T,
	query string,
	what string,
	buildArgs func(map[duckdb.UUID]T) []any,
) error {
	m = unseen(flushed, m)
	if len(m) == 0 {
		return nil
	}
	if err := execArgs(ctx, conn, query, buildArgs(m), what); err != nil {
		return err
	}
	mark(flushed, m)
	return nil
}

// attributesUpsert is static, unlike the per-batch `values (...), (...), ...`
// text it replaced: the arrays vary, the query never does, so DuckDB can
// actually prepare it once instead of replanning on every distinct row count.
const attributesUpsert = `insert into attributes (id, key, value, type, scope)
	select unnest(?::varchar[])::uuid, unnest(?::varchar[]), unnest(?::varchar[]), unnest(?::varchar[])::attr_type, unnest(?::varchar[])
	on conflict (id) do nothing`

func (d *Dictionary) flushAttributes(ctx context.Context, conn driver.Conn) error {
	return flushRows(ctx, conn, d.flushed, d.attributes, attributesUpsert, "attributes",
		func(rows map[duckdb.UUID]Attribute) []any {
			ids := make([]string, 0, len(rows))
			keys := make([]string, 0, len(rows))
			values := make([]string, 0, len(rows))
			types := make([]string, 0, len(rows))
			scopes := make([]string, 0, len(rows))
			for _, a := range rows {
				ids = append(ids, formatUUID(a.ID))
				keys = append(keys, a.Key)
				values = append(values, a.Value)
				types = append(types, a.Type)
				scopes = append(scopes, a.Scope)
			}
			return []any{ids, keys, values, types, scopes}
		})
}

const resourcesUpsert = `insert into resources (id, attribute_ids, dropped_attributes_count)
	select unnest(?::varchar[])::uuid, unnest(?::varchar[][])::uuid[], unnest(?::uinteger[])
	on conflict (id) do nothing`

func (d *Dictionary) flushResources(ctx context.Context, conn driver.Conn) error {
	return flushRows(ctx, conn, d.flushed, d.resources, resourcesUpsert, "resources",
		func(rows map[duckdb.UUID]Resource) []any {
			ids := make([]string, 0, len(rows))
			attributeIDs := make([][]string, 0, len(rows))
			dropped := make([]uint32, 0, len(rows))
			for _, r := range rows {
				ids = append(ids, formatUUID(r.ID))
				attributeIDs = append(attributeIDs, formatUUIDs(r.AttributeIDs))
				dropped = append(dropped, r.Dropped)
			}
			return []any{ids, attributeIDs, dropped}
		})
}

const scopesUpsert = `insert into scopes (id, name, version, attribute_ids, dropped_attributes_count)
	select unnest(?::varchar[])::uuid, unnest(?::varchar[]), unnest(?::varchar[]), unnest(?::varchar[][])::uuid[], unnest(?::uinteger[])
	on conflict (id) do nothing`

func (d *Dictionary) flushScopes(ctx context.Context, conn driver.Conn) error {
	return flushRows(ctx, conn, d.flushed, d.scopes, scopesUpsert, "scopes",
		func(rows map[duckdb.UUID]Scope) []any {
			ids := make([]string, 0, len(rows))
			names := make([]string, 0, len(rows))
			versions := make([]string, 0, len(rows))
			attributeIDs := make([][]string, 0, len(rows))
			dropped := make([]uint32, 0, len(rows))
			for _, sc := range rows {
				ids = append(ids, formatUUID(sc.ID))
				names = append(names, sc.Name)
				versions = append(versions, sc.Version)
				attributeIDs = append(attributeIDs, formatUUIDs(sc.AttributeIDs))
				dropped = append(dropped, sc.Dropped)
			}
			return []any{ids, names, versions, attributeIDs, dropped}
		})
}

// formatUUIDs renders each id in its canonical text form, for binding as a
// `varchar[]` element of the `varchar[][]` parameter an attribute_ids column
// unnests and casts back to uuid[]. A nil ids (an owner with no attributes)
// renders as an empty, non-nil []string, which the driver round-trips as SQL
// `[]` rather than NULL -- see NonNil below for why that distinction matters.
func formatUUIDs(ids []duckdb.UUID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = formatUUID(id)
	}
	return out
}

// NonNil normalises a nil id slice to an empty one.
//
// AttributeSet returns nil for an empty attribute map, but every
// attribute_ids column is NOT NULL: an owner with no attributes stores an
// empty array, not SQL NULL. Keeping the columns NOT NULL means every read
// path can join without a null guard, and there is no distinction to draw
// between "no attributes" and "no row".
func NonNil(ids []duckdb.UUID) []duckdb.UUID {
	if ids == nil {
		return []duckdb.UUID{}
	}
	return ids
}

// uuidList renders ids as a DuckDB list literal body.
//
// UUIDs cross the parameter boundary as text throughout this file, cast back
// with ?::uuid. The duckdb driver accepts duckdb.UUID as an *appender* value
// but not as a query parameter -- binding one fails with "unsupported data
// type" -- which is the same reason metrics.go passes uuid.NewString() to its
// stream upsert.
func uuidList(ids []duckdb.UUID) string {
	if len(ids) == 0 {
		return "[]"
	}
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = "'" + formatUUID(id) + "'"
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// FormatUUID renders the canonical 8-4-4-4-12 form. Exported because metrics
// builds its own metric_series insert and has to bind ids the same way.
func FormatUUID(id duckdb.UUID) string { return formatUUID(id) }

// UUIDListLiteral renders ids as a DuckDB list literal, for callers outside
// this package building their own inserts against a uuid[] column.
func UUIDListLiteral(ids []duckdb.UUID) string { return uuidList(ids) }

// formatUUID renders the canonical 8-4-4-4-12 form.
func formatUUID(id duckdb.UUID) string {
	h := uuidString(id)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

// parseUUID is the inverse of formatUUID, for ids read back from the database
// rather than computed here: LoadFlushedIDs warming the cache from what is
// already on disk, and SweepOrphans reading back exactly which ids a delete
// removed.
//
// duckdb.UUID and uuid.UUID are both plain [16]byte, so the conversion is
// direct -- same idiom metrics.go's decodeStreamID uses for a uuid column
// scanned as text.
func parseUUID(s string) (duckdb.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return duckdb.UUID{}, err
	}
	return duckdb.UUID(id), nil
}

func execArgs(ctx context.Context, conn driver.Conn, query string, args []any, what string) error {
	dconn, ok := conn.(*duckdb.Conn)
	if !ok {
		return fmt.Errorf("dictionary flush: %w: connection is not a *duckdb.Conn", ErrIngestInternal)
	}
	named, err := prepareNamedValues(dconn, args)
	if err != nil {
		return fmt.Errorf("dictionary flush %s: %w: %w", what, ErrIngestInternal, err)
	}
	if _, err := dconn.ExecContext(ctx, query, named); err != nil {
		return fmt.Errorf("dictionary flush %s: %w: %w", what, ErrIngestInternal, err)
	}
	return nil
}

// prepareNamedValues converts values through the driver's own checker, matching
// how metrics.go prepares its stream upsert arguments.
func prepareNamedValues(dconn *duckdb.Conn, args []any) ([]driver.NamedValue, error) {
	out := make([]driver.NamedValue, 0, len(args))
	for i, a := range args {
		nv := driver.NamedValue{Ordinal: i + 1, Value: a}
		if err := dconn.CheckNamedValue(&nv); err != nil {
			converted, cerr := driver.DefaultParameterConverter.ConvertValue(a)
			if cerr != nil {
				return nil, cerr
			}
			nv.Value = converted
		}
		out = append(out, nv)
	}
	return out, nil
}
