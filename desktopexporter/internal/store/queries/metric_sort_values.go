package queries

import (
	"database/sql/driver"
	"encoding/binary"
	"math"
	"math/bits"

	parquetvariant "github.com/apache/arrow-go/v18/parquet/variant"
	"github.com/duckdb/duckdb-go/v2"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

// Numeric keys compare int64 and binary64 without rounding integers through
// double or decimal. Normalize the exact significand to 64 bits and order its
// binary exponent first. Signed zero shares a numeric key; typed display and
// signature keys distinguish it later. NaN has one deterministic final slot.
func metricNumericSortKey(value any) []byte {
	var negative bool
	var significand uint64
	var exponent int
	switch v := value.(type) {
	case int8:
		return metricNumericSortKey(int64(v))
	case int16:
		return metricNumericSortKey(int64(v))
	case int32:
		return metricNumericSortKey(int64(v))
	case int64:
		if v == 0 {
			return []byte{2}
		}
		negative = v < 0
		magnitude := uint64(v)
		if negative {
			magnitude = uint64(-(v + 1)) + 1
		}
		exponent = bits.Len64(magnitude) - 1
		significand = magnitude << (63 - exponent)
	case float64:
		if math.IsNaN(v) {
			return []byte{5}
		}
		if math.IsInf(v, -1) {
			return []byte{0}
		}
		if math.IsInf(v, 1) {
			return []byte{4}
		}
		if v == 0 {
			return []byte{2}
		}
		raw := math.Float64bits(v)
		negative = raw>>63 != 0
		fraction := raw & ((1 << 52) - 1)
		encodedExponent := int((raw >> 52) & 0x7ff)
		if encodedExponent == 0 {
			exponent = bits.Len64(fraction) - 1 - 1074
			significand = fraction << (64 - bits.Len64(fraction))
		} else {
			exponent = encodedExponent - 1023
			significand = (fraction | (1 << 52)) << 11
		}
	default:
		return nil
	}
	key := make([]byte, 11)
	key[0] = 3
	binary.BigEndian.PutUint16(key[1:3], uint16(exponent+1074))
	binary.BigEndian.PutUint64(key[3:], significand)
	if negative {
		key[0] = 1
		for i := 1; i < len(key); i++ {
			key[i] = ^key[i]
		}
	}
	return key
}

type metricSortInfo struct{ config duckdb.ScalarFuncConfig }

func (f *metricSortInfo) Config() duckdb.ScalarFuncConfig { return f.config }
func (f *metricSortInfo) Executor() duckdb.ScalarFuncExecutor {
	return duckdb.ScalarFuncExecutor{RowExecutor: func(args []driver.Value) (any, error) {
		value, err := parquetvariant.New(args[0].([]byte), args[1].([]byte))
		if err != nil {
			return nil, err
		}
		label, signature, kind, err := metricLabelValue(value)
		if err != nil {
			return nil, err
		}
		var numeric any
		if key := metricNumericSortKey(value.Value()); key != nil {
			numeric = key
		}
		return map[string]any{
			"label": label, "signature": "[" + quoteMetricLabel(signature) + "]",
			"kind": kind, "numericKey": numeric,
		}, nil
	}}
}

func metricSortInfoType(text, blob duckdb.TypeInfo) (duckdb.TypeInfo, error) {
	var fields []duckdb.StructEntry
	for _, name := range []string{"label", "signature", "kind", "numericKey"} {
		typ := text
		if name == "numericKey" {
			typ = blob
		}
		field, err := duckdb.NewStructEntry(typ, name)
		if err != nil {
			return nil, err
		}
		fields = append(fields, field)
	}
	return duckdb.NewStructInfo(fields[0], fields[1:]...)
}

// DuckDB's bundled ICU collation does not enable numeric text collation through
// locale options. Supply English UCA keys explicitly; SQL still performs ORDER BY.
type metricTextSortKey struct{ config duckdb.ScalarFuncConfig }

func (f *metricTextSortKey) Config() duckdb.ScalarFuncConfig { return f.config }
func (f *metricTextSortKey) Executor() duckdb.ScalarFuncExecutor {
	return duckdb.ScalarFuncExecutor{RowExecutor: func(args []driver.Value) (any, error) {
		var options []collate.Option
		if args[1].(bool) {
			options = append(options, collate.Numeric)
		}
		collator := collate.New(language.English, options...)
		var buffer collate.Buffer
		return collator.KeyFromString(&buffer, args[0].(string)), nil
	}}
}
