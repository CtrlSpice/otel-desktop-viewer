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

// Home name sorting uses the complete explicit-Copy spelling, never a preview.
// This encoder is only called for name sorting, not for frequency pages.
type variantHomeText struct{ config duckdb.ScalarFuncConfig }

// HomeCopyQuote matches JSON.stringify, including lowercase control escapes and
// literal printable Unicode. Exported for the server's existing browser colour
// hash input; it does not encode or reconstruct value identities.
func HomeCopyQuote(text string) string {
	var out strings.Builder
	out.WriteByte('"')
	for _, r := range text {
		switch r {
		case '"', '\\':
			out.WriteByte('\\')
			out.WriteRune(r)
		case '\b':
			out.WriteString(`\b`)
		case '\t':
			out.WriteString(`\t`)
		case '\n':
			out.WriteString(`\n`)
		case '\f':
			out.WriteString(`\f`)
		case '\r':
			out.WriteString(`\r`)
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

func (f *variantHomeText) Config() duckdb.ScalarFuncConfig { return f.config }
func (f *variantHomeText) Executor() duckdb.ScalarFuncExecutor {
	return duckdb.ScalarFuncExecutor{RowExecutor: func(args []driver.Value) (any, error) {
		v, err := parquetvariant.New(args[0].([]byte), args[1].([]byte))
		if err != nil {
			return nil, err
		}
		return homeValueText(v, 0, true)
	}}
}

func homeValueText(v parquetvariant.Value, depth int, top bool) (string, error) {
	indent := strings.Repeat("  ", depth+1)
	switch value := v.Value().(type) {
	case nil:
		return "null", nil
	case string:
		if top {
			return value, nil
		}
		return HomeCopyQuote(value), nil
	case []byte:
		text := base64.StdEncoding.EncodeToString(value)
		if top {
			return text, nil
		}
		return HomeCopyQuote(text), nil
	case bool:
		return strconv.FormatBool(value), nil
	case int8:
		return strconv.FormatInt(int64(value), 10), nil
	case int16:
		return strconv.FormatInt(int64(value), 10), nil
	case int32:
		return strconv.FormatInt(int64(value), 10), nil
	case int64:
		return strconv.FormatInt(value, 10), nil
	case float64:
		switch {
		case math.IsNaN(value):
			return `"NaN"`, nil
		case math.IsInf(value, 1):
			return `"Infinity"`, nil
		case math.IsInf(value, -1):
			return `"-Infinity"`, nil
		case value == 0 && math.Signbit(value):
			return "-0", nil
		}
		format := byte('f')
		if value != 0 && (math.Abs(value) < 1e-6 || math.Abs(value) >= 1e21) {
			format = 'e'
		}
		text := strconv.FormatFloat(value, format, -1, 64)
		text = strings.ReplaceAll(strings.ReplaceAll(text, "e-0", "e-"), "e+0", "e+")
		return text, nil
	case parquetvariant.ArrayValue:
		items := make([]string, 0, value.Len())
		for i := uint32(0); i < value.Len(); i++ {
			child, err := value.Value(i)
			if err != nil {
				return "", err
			}
			text, err := homeValueText(child, depth+1, false)
			if err != nil {
				return "", err
			}
			items = append(items, indent+text)
		}
		if len(items) == 0 {
			return "[]", nil
		}
		return "[\n" + strings.Join(items, ",\n") + "\n" + strings.Repeat("  ", depth) + "]", nil
	case parquetvariant.ObjectValue:
		items := make([]string, 0, value.NumElements())
		for i := uint32(0); i < value.NumElements(); i++ {
			field, err := value.FieldAt(i)
			if err != nil {
				return "", err
			}
			text, err := homeValueText(field.Value, depth+1, false)
			if err != nil {
				return "", err
			}
			items = append(items, indent+HomeCopyQuote(field.Key)+": "+text)
		}
		if len(items) == 0 {
			return "{}", nil
		}
		return "{\n" + strings.Join(items, ",\n") + "\n" + strings.Repeat("  ", depth) + "}", nil
	default:
		return "", fmt.Errorf("unsupported Home value %v", v.Type())
	}
}
