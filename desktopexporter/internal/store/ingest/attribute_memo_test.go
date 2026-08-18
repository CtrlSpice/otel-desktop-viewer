package ingest

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

func mapOf(build func(m pcommon.Map)) pcommon.Map {
	m := pcommon.NewMap()
	build(m)
	return m
}

// TestAttributeMemoAgreesWithDerivation is the memo's whole safety argument: it
// stands in front of a pure function, so for every input the cached answer must
// be the answer the derivation would have given. Each case is asked twice --
// once cold, once warm -- because only the second is served by the memo.
func TestAttributeMemoAgreesWithDerivation(t *testing.T) {
	cases := []struct {
		name  string
		attrs pcommon.Map
		scope string
	}{
		{"empty", mapOf(func(m pcommon.Map) {}), ScopeDatapoint},
		{"one string", mapOf(func(m pcommon.Map) { m.PutStr("k", "v") }), ScopeDatapoint},
		{"every scalar type", mapOf(func(m pcommon.Map) {
			m.PutStr("s", "text")
			m.PutInt("i", -42)
			m.PutDouble("d", 3.5)
			m.PutBool("b", true)
			m.PutEmpty("e")
		}), ScopeSpan},
		// Values that collide under a careless fingerprint: same bits read as a
		// different type, and numbers whose string forms differ from their bits.
		{"int and double sharing bits", mapOf(func(m pcommon.Map) {
			m.PutInt("n", 1)
			m.PutDouble("f", 1)
		}), ScopeDatapoint},
		{"negative zero", mapOf(func(m pcommon.Map) { m.PutDouble("z", negZero()) }), ScopeDatapoint},
		{"false is not empty", mapOf(func(m pcommon.Map) { m.PutBool("b", false) }), ScopeDatapoint},
		// Complex values take the uncached path; they must still be correct.
		{"nested map", mapOf(func(m pcommon.Map) {
			m.PutStr("plain", "x")
			m.PutEmptyMap("nested").PutStr("inner", "y")
		}), ScopeResource},
		{"slice", mapOf(func(m pcommon.Map) {
			m.PutEmptySlice("list").AppendEmpty().SetInt(7)
		}), ScopeResource},
		{"bytes", mapOf(func(m pcommon.Map) {
			m.PutEmptyBytes("raw").Append(1, 2, 3)
		}), ScopeLog},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			wantRows, wantIDs := attributeSetUncached(c.attrs, c.scope)
			if c.attrs.Len() == 0 {
				wantRows, wantIDs = nil, nil
			}
			for _, pass := range []string{"cold", "warm"} {
				rows, ids := AttributeSet(c.attrs, c.scope)
				assert.Equalf(t, wantRows, rows, "%s pass rows", pass)
				assert.Equalf(t, wantIDs, ids, "%s pass ids", pass)
			}
		})
	}
}

func negZero() float64 { z := 0.0; return -z }

// TestAttributeMemoSeparatesScopes covers the mistake that would be silent:
// the same labels under two scopes are different dictionary entries, and a memo
// keyed on labels alone would serve one for the other.
func TestAttributeMemoSeparatesScopes(t *testing.T) {
	attrs := mapOf(func(m pcommon.Map) { m.PutStr("http.method", "GET") })

	_, spanIDs := AttributeSet(attrs, ScopeSpan)
	_, logIDs := AttributeSet(attrs, ScopeLog)
	_, spanAgain := AttributeSet(attrs, ScopeSpan)

	require.Len(t, spanIDs, 1)
	assert.NotEqual(t, spanIDs[0], logIDs[0],
		"scope is part of dictionary identity, so these are different rows")
	assert.Equal(t, spanIDs, spanAgain,
		"and the warm read must return the span answer, not whichever was cached last")
}

// TestAttributeMemoConfirmsOnCollision forces two different sets into one
// bucket and asserts each still resolves to its own ids.
//
// The fingerprint is 64-bit and a natural collision is not reachable in a test,
// so the entry is planted directly. That is the point: the memo must not trust
// the fingerprint, and the exact comparison is what makes a collision cost a
// wasted slot instead of attaching another series' attributes.
func TestAttributeMemoConfirmsOnCollision(t *testing.T) {
	ResetAttributeMemo()
	t.Cleanup(ResetAttributeMemo)

	mine := mapOf(func(m pcommon.Map) { m.PutStr("pod", "a") })
	theirs := mapOf(func(m pcommon.Map) { m.PutStr("pod", "b") })

	wantMine, wantMineIDs := attributeSetUncached(mine, ScopeDatapoint)
	theirsRows, theirsIDs := AttributeSet(theirs, ScopeDatapoint)

	// Plant "theirs" under the fingerprint "mine" will compute.
	fp, ok := fingerprint(mine, ScopeDatapoint)
	require.True(t, ok)
	attributeMemo.mu.Lock()
	attributeMemo.buckets[fp] = append(attributeMemo.buckets[fp], memoEntry{
		scope: ScopeDatapoint,
		raw:   rawOf(theirs),
		rows:  theirsRows,
		ids:   theirsIDs,
	})
	attributeMemo.mu.Unlock()

	rows, ids := AttributeSet(mine, ScopeDatapoint)
	assert.Equal(t, wantMine, rows, "the colliding entry must be rejected, not returned")
	assert.Equal(t, wantMineIDs, ids)
	assert.NotEqual(t, theirsIDs, ids, "pod=b's ids must never answer for pod=a")
}

// TestAttributeMemoHitsRepeatedSets is the case the change rests on: the
// reference capture's 294,607 datapoints hold 89 distinct label sets, so all
// but the first 89 derivations should be served from the memo.
func TestAttributeMemoHitsRepeatedSets(t *testing.T) {
	ResetAttributeMemo()
	t.Cleanup(ResetAttributeMemo)

	const distinct = 89
	const reports = 200
	sets := make([]pcommon.Map, distinct)
	for i := range distinct {
		sets[i] = mapOf(func(m pcommon.Map) {
			m.PutStr("driver", fmt.Sprintf("D%02d", i))
			m.PutStr("session", "race")
			m.PutInt("lap", int64(i))
		})
	}
	for range reports {
		for _, s := range sets {
			AttributeSet(s, ScopeDatapoint)
		}
	}

	hits, misses, entries := AttributeMemoStats()
	assert.Equal(t, uint64(distinct), misses,
		"each distinct set is derived once and only once")
	assert.Equal(t, uint64(distinct*(reports-1)), hits)
	assert.Equal(t, distinct, entries)
}

// TestAttributeMemoResetsAtCap keeps the memo from pinning memory under a
// high-cardinality label, which no fixed size defeats.
func TestAttributeMemoResetsAtCap(t *testing.T) {
	ResetAttributeMemo()
	t.Cleanup(ResetAttributeMemo)

	for i := range attributeMemoCap + 10 {
		AttributeSet(mapOf(func(m pcommon.Map) {
			m.PutStr("request.id", fmt.Sprintf("r%d", i))
		}), ScopeSpan)
	}
	_, _, entries := AttributeMemoStats()
	assert.LessOrEqual(t, entries, attributeMemoCap,
		"a unique label per request must not grow the memo without bound")
	assert.Positive(t, entries, "and the memo must still be usable after a reset")
}

// TestAttributeMemoFingerprintSpreads guards the half of the design the
// correctness tests cannot see.
//
// The fingerprint is only a bucketing hint: exact comparison is what makes an
// answer right, so a fingerprint that ignored the values entirely -- or the
// scope -- would still return correct results and pass every test above. It
// would also drop every set into one bucket and turn each lookup into a linear
// scan of the whole memo, which at the cap is slower than deriving the answer
// and is exactly the cost the memo exists to remove.
func TestAttributeMemoFingerprintSpreads(t *testing.T) {
	ResetAttributeMemo()
	t.Cleanup(ResetAttributeMemo)

	// Same keys and types throughout, so only the values distinguish them --
	// and only *numeric* values, because a string value reaches the hash
	// through a different branch. A fixture that varied a string too would
	// still spread if the numeric branch were dropped entirely.
	const distinct = 256
	for i := range distinct {
		AttributeSet(mapOf(func(m pcommon.Map) {
			m.PutStr("pod", "fixed")
			m.PutInt("shard", int64(i))
			m.PutDouble("weight", float64(i)/8)
		}), ScopeDatapoint)
	}

	attributeMemo.mu.Lock()
	buckets := len(attributeMemo.buckets)
	widest := 0
	for _, b := range attributeMemo.buckets {
		if len(b) > widest {
			widest = len(b)
		}
	}
	attributeMemo.mu.Unlock()

	assert.GreaterOrEqual(t, buckets, distinct*3/4,
		"sets differing only in value must land in distinct buckets, or lookup "+
			"degrades to a scan of everything the memo holds")
	assert.LessOrEqual(t, widest, 4,
		"and no bucket should collect a meaningful share of them")

	// Scope is part of dictionary identity, so it has to reach the fingerprint
	// too -- otherwise every scope's copy of a label set stacks in one bucket.
	before := buckets
	attrs := mapOf(func(m pcommon.Map) { m.PutStr("http.method", "GET") })
	for _, scope := range []string{ScopeSpan, ScopeLog, ScopeDatapoint, ScopeResource} {
		AttributeSet(attrs, scope)
	}
	attributeMemo.mu.Lock()
	after := len(attributeMemo.buckets)
	attributeMemo.mu.Unlock()
	assert.Equal(t, before+4, after,
		"one identical label set under four scopes is four entries in four buckets")
}
