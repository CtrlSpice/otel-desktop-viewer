package util

import (
	"math"
	"testing"

	"go.opentelemetry.io/collector/pdata/pcommon"
)

func TestEncodeValuePreservesKindsAndDoubleBits(t *testing.T) {
	v := pcommon.NewValueMap()
	m := v.Map()
	m.PutEmpty("empty")
	m.PutStr("string", `{"looks":"like json"}`)
	m.PutBool("bool", true)
	m.PutInt("int", math.MinInt64)
	m.PutDouble("negativeZero", math.Float64frombits(0x8000000000000000))
	m.PutDouble("nan", math.Float64frombits(0x7ff8000000000001))
	m.PutEmptyBytes("bytes").FromRaw([]byte{0xfb, 0xff})
	slice := m.PutEmptySlice("slice")
	slice.AppendEmpty().SetStr("one")
	slice.AppendEmpty().SetInt(2)

	got, err := EncodeValue(v)
	if err != nil {
		t.Fatalf("EncodeValue() error = %v", err)
	}
	want := `{"kind":"map","value":[{"key":"bool","value":{"kind":"bool","value":true}},{"key":"bytes","value":{"kind":"bytes","value":"+/8="}},{"key":"empty","value":{"kind":"empty","value":null}},{"key":"int","value":{"kind":"int64","value":"-9223372036854775808"}},{"key":"nan","value":{"kind":"double","value":"0x7ff8000000000001"}},{"key":"negativeZero","value":{"kind":"double","value":"0x8000000000000000"}},{"key":"slice","value":{"kind":"array","value":[{"kind":"string","value":"one"},{"kind":"int64","value":"2"}]}},{"key":"string","value":{"kind":"string","value":"{\"looks\":\"like json\"}"}}]}`
	if string(got) != want {
		t.Errorf("EncodeValue() = %s, want %s", got, want)
	}
}
