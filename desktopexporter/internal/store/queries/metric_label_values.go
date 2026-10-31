package queries

import (
	"database/sql/driver"
	"encoding/base64"
	"fmt"
	"math"
	"strconv"
	"strings"

	parquetvariant "github.com/apache/arrow-go/v18/parquet/variant"
	"github.com/duckdb/duckdb-go/v2"
)

// These strings mirror attributeValueLabel/valueSignature. They are used only
// to preserve the existing minimum-column cost/tie policy. SQL equality uses
// the canonical typed wire value, independently of this display representation.
type metricLabelInfo struct{ config duckdb.ScalarFuncConfig }

func (f *metricLabelInfo) Config() duckdb.ScalarFuncConfig { return f.config }
func (f *metricLabelInfo) Executor() duckdb.ScalarFuncExecutor {
	return duckdb.ScalarFuncExecutor{RowExecutor: func(args []driver.Value) (any, error) {
		value, err := parquetvariant.New(args[0].([]byte), args[1].([]byte))
		if err != nil {
			return nil, err
		}
		label, signature, kind, err := metricLabelValue(value)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"label": label, "kind": kind,
			"labelLength":     int64(metricLabelLength(label)),
			"signatureLength": int64(metricLabelLength("[" + quoteMetricLabel(signature) + "]")),
			"kindLength":      int64(metricLabelLength(kind)),
		}, nil
	}}
}

func metricLabelInfoType(text duckdb.TypeInfo) (duckdb.TypeInfo, error) {
	integer, err := duckdb.NewTypeInfo(duckdb.TYPE_BIGINT)
	if err != nil {
		return nil, err
	}
	var fields []duckdb.StructEntry
	for _, field := range []string{"label", "kind", "labelLength", "signatureLength", "kindLength"} {
		kind := integer
		if field == "label" || field == "kind" {
			kind = text
		}
		entry, err := duckdb.NewStructEntry(kind, field)
		if err != nil {
			return nil, err
		}
		fields = append(fields, entry)
	}
	return duckdb.NewStructInfo(fields[0], fields[1:]...)
}

func quoteMetricLabel(text string) string {
	var out strings.Builder
	out.WriteByte('"')
	for _, r := range text {
		switch r {
		case '"', '\\':
			out.WriteByte('\\')
			out.WriteRune(r)
		case '\b':
			out.WriteString(`\b`)
		case '\f':
			out.WriteString(`\f`)
		case '\n':
			out.WriteString(`\n`)
		case '\r':
			out.WriteString(`\r`)
		case '\t':
			out.WriteString(`\t`)
		default:
			if r < 0x20 {
				fmt.Fprintf(&out, `\u%04x`, r)
			} else {
				out.WriteRune(r)
			}
		}
	}
	out.WriteByte('"')
	return out.String()
}

func metricLabelNumber(value float64) string {
	switch {
	case math.IsNaN(value):
		return "NaN"
	case math.IsInf(value, 1):
		return "Infinity"
	case math.IsInf(value, -1):
		return "-Infinity"
	case value == 0 && math.Signbit(value):
		return "-0"
	case value == 0:
		return "0"
	case math.Abs(value) >= 1e-6 && math.Abs(value) < 1e21:
		return strconv.FormatFloat(value, 'f', -1, 64)
	default:
		text := strconv.FormatFloat(value, 'e', -1, 64)
		parts := strings.Split(text, "e")
		exponent, _ := strconv.Atoi(parts[1])
		if exponent >= 0 {
			return parts[0] + "e+" + strconv.Itoa(exponent)
		}
		return parts[0] + "e" + strconv.Itoa(exponent)
	}
}

func metricLabelValue(value parquetvariant.Value) (label, signature, kind string, err error) {
	var payload string
	switch raw := value.Value().(type) {
	case nil:
		kind, label, payload = "empty", "null", "null"
	case string:
		kind, label = "string", raw
		payload = quoteMetricLabel(label)
	case []byte:
		kind, label = "bytes", base64.StdEncoding.EncodeToString(raw)
		payload = quoteMetricLabel(label)
	case bool:
		kind, label = "bool", strconv.FormatBool(raw)
		payload = label
	case int8, int16, int32, int64:
		kind, label = "int64", fmt.Sprint(raw)
		payload = quoteMetricLabel(label)
	case float64:
		kind, label = "double", metricLabelNumber(raw)
		payload = quoteMetricLabel(label)
	case parquetvariant.ArrayValue:
		kind = "array"
		labels, signatures := make([]string, raw.Len()), make([]string, raw.Len())
		for i := range labels {
			child, err := raw.Value(uint32(i))
			if err != nil {
				return "", "", "", err
			}
			childLabel, childSignature, _, err := metricLabelValue(child)
			if err != nil {
				return "", "", "", err
			}
			labels[i], signatures[i] = childLabel, quoteMetricLabel(childSignature)
		}
		label, payload = "["+strings.Join(labels, ", ")+"]", "["+strings.Join(signatures, ",")+"]"
	case parquetvariant.ObjectValue:
		kind = "map"
		keys := make([]string, 0, raw.NumElements())
		for key := range raw.Values() {
			keys = append(keys, key)
		}
		// Native/wire attribute maps are canonically key-ordered.
		sortMetricLabelKeys(keys)
		labels, signatures := make([]string, len(keys)), make([]string, len(keys))
		for i, key := range keys {
			child, err := raw.ValueByKey(key)
			if err != nil {
				return "", "", "", err
			}
			childLabel, childSignature, _, err := metricLabelValue(child.Value)
			if err != nil {
				return "", "", "", err
			}
			labels[i] = key + ": " + childLabel
			signatures[i] = "[" + quoteMetricLabel(key) + "," + quoteMetricLabel(childSignature) + "]"
		}
		label, payload = "{"+strings.Join(labels, ", ")+"}", "["+strings.Join(signatures, ",")+"]"
	default:
		return "", "", "", fmt.Errorf("non-OTel metric label type %v", value.Type())
	}
	return label, "[" + quoteMetricLabel(kind) + "," + payload + "]", kind, nil
}

func metricLabelLength(text string) int {
	length := 0
	for _, r := range text {
		length++
		if r > 0xffff {
			length++
		}
	}
	return length
}
