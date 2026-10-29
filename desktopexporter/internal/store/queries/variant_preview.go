package queries

import (
	"database/sql/driver"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/util"
	parquetvariant "github.com/apache/arrow-go/v18/parquet/variant"
	"github.com/duckdb/duckdb-go/v2"
)

// variantPreview formats a bounded display value in the SQL projection. It opens
// at most three children and reads only the size of child containers; neither
// pdata trees nor recursive JSON documents are built for a summary.
type variantPreview struct {
	config duckdb.ScalarFuncConfig
	column bool
}

func (f *variantPreview) Config() duckdb.ScalarFuncConfig { return f.config }
func (f *variantPreview) Executor() duckdb.ScalarFuncExecutor {
	return duckdb.ScalarFuncExecutor{RowExecutor: func(args []driver.Value) (any, error) {
		v, err := parquetvariant.New(args[0].([]byte), args[1].([]byte))
		if err != nil {
			return nil, err
		}
		return previewVariant(v, true, f.column)
	}}
}

var previewWhitespace = regexp.MustCompile(`[\t\n\f\r ]+`)

func previewText(text string, limit int, ellipsisWithinLimit bool) string {
	count, previous := 0, 0
	for offset := range text {
		if count == limit {
			if ellipsisWithinLimit {
				offset = previous
			}
			return text[:offset] + "…"
		}
		previous = offset
		count++
	}
	return text
}

// Match SQL json_quote: escape JSON controls, not HTML or printable Unicode.
func quotePreview(text string) string {
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
				fmt.Fprintf(&out, `\u%04X`, r)
			} else {
				out.WriteRune(r)
			}
		}
	}
	out.WriteByte('"')
	return out.String()
}

func previewVariant(v parquetvariant.Value, top, column bool) (string, error) {
	switch value := v.Value().(type) {
	case nil:
		if column {
			return "null", nil
		}
		return "empty", nil
	case string:
		if column && top {
			if value == "" {
				return `""`, nil
			}
			return previewText(value, 300, true), nil
		}
		text := previewWhitespace.ReplaceAllString(value, " ")
		if top {
			return previewText(text, 300, true), nil
		}
		return quotePreview(previewText(text, 48, false)), nil
	case []byte:
		if column {
			text := base64.StdEncoding.EncodeToString(value[:min(len(value), 36)])
			if len(value) > 36 {
				text += "…"
			}
			return quotePreview(text), nil
		}
		text := base64.StdEncoding.EncodeToString(value[:min(len(value), 36)])
		if len(value) > 36 {
			text += "…"
		}
		return "bytes (base64): " + text, nil
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
		value = util.CanonicalDouble(value)
		if column {
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
		}
		if math.IsNaN(value) || math.IsInf(value, 0) || value == 0 && math.Signbit(value) {
			return fmt.Sprintf("0x%016x", math.Float64bits(value)), nil
		}
		text, err := json.Marshal(value)
		return string(text), err
	case parquetvariant.ArrayValue:
		if !top {
			return fmt.Sprintf("array (%d items)", value.Len()), nil
		}
		items := make([]string, 0, 4)
		for i := uint32(0); i < min(value.Len(), 3); i++ {
			child, err := value.Value(i)
			if err != nil {
				return "", err
			}
			text, err := previewVariant(child, false, column)
			if err != nil {
				return "", err
			}
			items = append(items, text)
		}
		if value.Len() > 3 {
			items = append(items, fmt.Sprintf("… (+%d items)", value.Len()-3))
		}
		return "[" + strings.Join(items, ", ") + "]", nil
	case parquetvariant.ObjectValue:
		if !top {
			return fmt.Sprintf("object (%d fields)", value.NumElements()), nil
		}
		items := make([]string, 0, 4)
		// variant_normalize supplies sorted fields, matching canonical wire order.
		for i := uint32(0); i < min(value.NumElements(), 3); i++ {
			field, err := value.FieldAt(i)
			if err != nil {
				return "", err
			}
			text, err := previewVariant(field.Value, false, column)
			if err != nil {
				return "", err
			}
			items = append(items, previewText(field.Key, 32, false)+": "+text)
		}
		if value.NumElements() > 3 {
			items = append(items, fmt.Sprintf("… (+%d fields)", value.NumElements()-3))
		}
		return "{" + strings.Join(items, ", ") + "}", nil
	default:
		return "", fmt.Errorf("non-OTel VARIANT type %v", v.Type())
	}
}
