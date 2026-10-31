package queries

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/util"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

// encodeOTLPValue writes AnyValue directly from the typed decoder. Integers use
// decimal strings; bytes use base64; maps have deterministic key order. Empty
// values and empty containers retain their different OTLP representations.
func encodeOTLPValue(v pcommon.Value) ([]byte, error) {
	return appendOTLPValue(nil, v)
}

func appendOTLPValue(dst []byte, v pcommon.Value) ([]byte, error) {
	switch v.Type() {
	case pcommon.ValueTypeEmpty:
		return append(dst, "{}"...), nil
	case pcommon.ValueTypeStr:
		dst = append(dst, `{"stringValue":`...)
		dst = appendJSONString(dst, v.Str())
	case pcommon.ValueTypeBool:
		dst = append(dst, `{"boolValue":`...)
		dst = strconv.AppendBool(dst, v.Bool())
	case pcommon.ValueTypeInt:
		dst = append(dst, `{"intValue":"`...)
		dst = strconv.AppendInt(dst, v.Int(), 10)
		dst = append(dst, '"')
	case pcommon.ValueTypeDouble:
		dst = append(dst, `{"doubleValue":`...)
		n := v.Double()
		switch {
		case math.IsNaN(n):
			dst = append(dst, `"NaN"`...)
		case math.IsInf(n, 1):
			dst = append(dst, `"Infinity"`...)
		case math.IsInf(n, -1):
			dst = append(dst, `"-Infinity"`...)
		case n == 0 && math.Signbit(n):
			dst = append(dst, "-0.0"...)
		default:
			dst = strconv.AppendFloat(dst, n, 'g', -1, 64)
		}
	case pcommon.ValueTypeBytes:
		dst = append(dst, `{"bytesValue":"`...)
		dst = base64.StdEncoding.AppendEncode(dst, v.Bytes().AsRaw())
		dst = append(dst, '"')
	case pcommon.ValueTypeSlice:
		dst = append(dst, `{"arrayValue":{"values":[`...)
		for i, child := range v.Slice().All() {
			if i > 0 {
				dst = append(dst, ',')
			}
			var err error
			dst, err = appendOTLPValue(dst, child)
			if err != nil {
				return nil, err
			}
		}
		return append(dst, "]}}"...), nil
	case pcommon.ValueTypeMap:
		dst = append(dst, `{"kvlistValue":{"values":[`...)
		values := util.LastValues(v.Map())
		keys := make([]string, 0, len(values))
		for key := range values {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for i, key := range keys {
			if i > 0 {
				dst = append(dst, ',')
			}
			dst = append(dst, `{"key":`...)
			dst = appendJSONString(dst, key)
			dst = append(dst, `,"value":`...)
			var err error
			dst, err = appendOTLPValue(dst, values[key])
			if err != nil {
				return nil, err
			}
			dst = append(dst, '}')
		}
		return append(dst, "]}}"...), nil
	default:
		return nil, fmt.Errorf("unsupported OpenTelemetry value kind %d", v.Type())
	}
	return append(dst, '}'), nil
}

func appendJSONString(dst []byte, s string) []byte {
	encoded, _ := json.Marshal(s) // A Go string always has a JSON encoding.
	return append(dst, encoded...)
}
