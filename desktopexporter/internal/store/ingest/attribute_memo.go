package ingest

import (
	"hash/maphash"
	"math"
	"sync"

	"github.com/duckdb/duckdb-go/v2"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

// The attribute-set memo: content to (rows, ids), for the duration of the
// process.
//
// AttributeSet is a pure function -- a sha256 per label, a sort, and the two
// slices that come out -- and we ran it once per datapoint. The reference
// capture's 294,607 datapoints hold 89 distinct label sets, so that is ~208ms
// and ~238MB of garbage per ingest spent rediscovering 89 answers. The
// repetition is across batches, not inside them: a batch holds one datapoint
// per series, so a per-batch memo would have nothing to hit.
//
// Package-level rather than threaded through the store, which is unusual here
// and deliberate. FlushedIDs is per-store because it asserts something about
// one database's contents and has to be invalidated when they change; this
// asserts that the same bytes hash to the same ids, which is true of every
// store in the process and can never stop being true. Scoping it per store
// would buy isolation nothing needs and cost a parameter on three Ingest
// signatures and their hundred-odd callers.
//
// The returned slices are shared with the memo and with every later caller
// holding the same set. Nothing mutates them today -- rows are read into the
// dictionary map, ids go to an appender -- and nothing should start.
//
// The trade, measured on an M4 Pro over five-label sets: the repeating shape
// goes from ~830ns and 808B per call to ~104ns and no allocation, while a
// stream whose every datapoint carries a unique label set -- a request id, say,
// which the memo can never serve -- costs ~955ns against ~830ns, about 15%
// more. That case is accepted rather than detected and switched off. A unique
// set per datapoint means a new dictionary row per datapoint too, so the write
// this memo sits in front of grows with the same N it fails on; adding a mode
// to save 15% of one function there would be tuning the smaller term, and the
// threshold that decided when to flip it would be one more thing to get
// wrong.
const (
	// Distinct sets held before the memo resets. The reference capture needs
	// 89; the headroom is for label sets we have not seen, not for a
	// high-cardinality label, which no size defeats.
	attributeMemoCap = 4096
)

type rawAttr struct {
	key string
	typ pcommon.ValueType
	str string
	// Int, Double and Bool all live here: the value's bits, so comparison is
	// one integer compare and the fingerprint needs no formatting.
	num uint64
}

type memoEntry struct {
	scope string
	raw   []rawAttr
	rows  []Attribute
	ids   []duckdb.UUID
}

var attributeMemo = struct {
	mu      sync.Mutex
	seed    maphash.Seed
	buckets map[uint64][]memoEntry
	n       int
	hits    uint64
	misses  uint64
}{
	seed:    maphash.MakeSeed(),
	buckets: map[uint64][]memoEntry{},
}

// fingerprint hashes a set's raw content without formatting or allocating.
//
// Reports false when the set holds a Map, Slice or Bytes value. Those reach
// their string form through fmt, which is neither cheap nor obviously stable to
// hash against, and datapoint labels are scalars in practice -- so they take
// the uncached path rather than putting a formatting call on this one.
//
// Order-sensitive, because confirmation below walks the map in the same order.
// A producer that reorders an otherwise identical set gets a miss, not a wrong
// answer: AttributeSet sorts by id, so the result is the same either way.
func fingerprint(attrs pcommon.Map, scope string) (uint64, bool) {
	var h maphash.Hash
	h.SetSeed(attributeMemo.seed)
	h.WriteString(scope)
	for k, v := range attrs.All() {
		h.WriteString(k)
		var n uint64
		switch v.Type() {
		case pcommon.ValueTypeStr:
			h.WriteString(v.Str())
		case pcommon.ValueTypeInt:
			n = uint64(v.Int())
		case pcommon.ValueTypeDouble:
			n = math.Float64bits(v.Double())
		case pcommon.ValueTypeBool:
			if v.Bool() {
				n = 1
			}
		case pcommon.ValueTypeEmpty:
		default:
			return 0, false
		}
		var b [9]byte
		b[0] = byte(v.Type())
		for i := range 8 {
			b[i+1] = byte(n >> (8 * i))
		}
		h.Write(b[:])
	}
	return h.Sum64(), true
}

// matches confirms a candidate entry holds exactly this set, so that a
// fingerprint collision costs a wasted slot rather than silently attaching
// another series' attributes. Walks the map without materialising it.
func matches(attrs pcommon.Map, scope string, e *memoEntry) bool {
	if e.scope != scope || len(e.raw) != attrs.Len() {
		return false
	}
	i := 0
	ok := true
	for k, v := range attrs.All() {
		r := e.raw[i]
		i++
		if r.key != k || r.typ != v.Type() {
			ok = false
			break
		}
		switch v.Type() {
		case pcommon.ValueTypeStr:
			if r.str != v.Str() {
				ok = false
			}
		case pcommon.ValueTypeInt:
			if r.num != uint64(v.Int()) {
				ok = false
			}
		case pcommon.ValueTypeDouble:
			if r.num != math.Float64bits(v.Double()) {
				ok = false
			}
		case pcommon.ValueTypeBool:
			var n uint64
			if v.Bool() {
				n = 1
			}
			if r.num != n {
				ok = false
			}
		}
		if !ok {
			break
		}
	}
	return ok
}

func rawOf(attrs pcommon.Map) []rawAttr {
	raw := make([]rawAttr, 0, attrs.Len())
	for k, v := range attrs.All() {
		r := rawAttr{key: k, typ: v.Type()}
		switch v.Type() {
		case pcommon.ValueTypeStr:
			r.str = v.Str()
		case pcommon.ValueTypeInt:
			r.num = uint64(v.Int())
		case pcommon.ValueTypeDouble:
			r.num = math.Float64bits(v.Double())
		case pcommon.ValueTypeBool:
			if v.Bool() {
				r.num = 1
			}
		}
		raw = append(raw, r)
	}
	return raw
}

// memoLookup returns a previously derived set, or false. The caller supplies
// the fingerprint so that a miss pays for hashing once rather than again on the
// way to memoStore -- which is the whole cost of the memo on a stream it can
// never help.
func memoLookup(fp uint64, attrs pcommon.Map, scope string) ([]Attribute, []duckdb.UUID, bool) {
	attributeMemo.mu.Lock()
	defer attributeMemo.mu.Unlock()
	for i := range attributeMemo.buckets[fp] {
		e := &attributeMemo.buckets[fp][i]
		if matches(attrs, scope, e) {
			attributeMemo.hits++
			return e.rows, e.ids, true
		}
	}
	attributeMemo.misses++
	return nil, nil, false
}

// memoStore records a derivation.
//
// At the cap the whole memo is dropped rather than evicted one entry at a time.
// LRU bookkeeping would sit on the ingest hot path to little end: the sets that
// matter report every interval and are re-admitted immediately, while a
// high-cardinality label defeats any fixed size anyway -- what a reset
// guarantees is that the memo cannot pin memory for sets that stopped
// reporting, which freezing on a full table would.
func memoStore(fp uint64, attrs pcommon.Map, scope string, rows []Attribute, ids []duckdb.UUID) {
	attributeMemo.mu.Lock()
	defer attributeMemo.mu.Unlock()
	if attributeMemo.n >= attributeMemoCap {
		attributeMemo.buckets = map[uint64][]memoEntry{}
		attributeMemo.n = 0
	}
	attributeMemo.buckets[fp] = append(attributeMemo.buckets[fp], memoEntry{
		scope: scope,
		raw:   rawOf(attrs),
		rows:  rows,
		ids:   ids,
	})
	attributeMemo.n++
}

// AttributeMemoStats reports hits, misses and how many distinct sets are held.
// For tests and benchmarks; the hit rate is the whole case for the memo.
func AttributeMemoStats() (hits, misses uint64, entries int) {
	attributeMemo.mu.Lock()
	defer attributeMemo.mu.Unlock()
	return attributeMemo.hits, attributeMemo.misses, attributeMemo.n
}

// ResetAttributeMemo empties the memo. For tests that measure it; ingest never
// needs it, since the mapping it caches cannot go stale.
func ResetAttributeMemo() {
	attributeMemo.mu.Lock()
	defer attributeMemo.mu.Unlock()
	attributeMemo.buckets = map[uint64][]memoEntry{}
	attributeMemo.n = 0
	attributeMemo.hits = 0
	attributeMemo.misses = 0
}
