package searchindex

import (
	"encoding/base64"
	"fmt"
	"math"
	"sort"
	"strconv"

	pv "github.com/apache/arrow-go/v18/parquet/variant"
)

// Metadata dictionary offsets are not identities. Hash typed contents instead,
// retaining float bits and canonical object key order without payload JSON.
func (o *Object) walkNative(v pv.Value, path []any, key string) (string, string, error) {
	var text, identity, kind string
	childPath := func(part any) []any { return append(append([]any(nil), path...), part) }
	switch x := v.Value().(type) {
	case nil:
		kind = "empty"
		text = "null"
		identity = digest("empty")
	case string:
		kind = "string"
		text = x
		identity = digest("string", x)
	case bool:
		kind = "bool"
		text = strconv.FormatBool(x)
		identity = digest("bool", text)
	case int8:
		kind = "int64"
		text = strconv.FormatInt(int64(x), 10)
		identity = digest("int64", text)
	case int16:
		kind = "int64"
		text = strconv.FormatInt(int64(x), 10)
		identity = digest("int64", text)
	case int32:
		kind = "int64"
		text = strconv.FormatInt(int64(x), 10)
		identity = digest("int64", text)
	case int64:
		kind = "int64"
		text = strconv.FormatInt(x, 10)
		identity = digest("int64", text)
	case float64:
		kind = "double"
		text = strconv.FormatFloat(x, 'g', -1, 64)
		identity = digest("double", strconv.FormatUint(math.Float64bits(x), 16))
	case []byte:
		kind = "bytes"
		text = base64.StdEncoding.EncodeToString(x)
		identity = digest("bytes", text)
	case pv.ArrayValue:
		kind = "array"
		children := []string{"array"}
		for i := uint32(0); i < x.Len(); i++ {
			child, err := x.Value(i)
			if err != nil {
				return "", "", err
			}
			_, id, err := o.walkNative(child, childPath(i), "")
			if err != nil {
				return "", "", err
			}
			children = append(children, id)
		}
		identity = digest(children...)
	case pv.ObjectValue:
		kind = "map"
		fields := map[string]pv.Value{}
		var keys []string
		for key, field := range x.Values() {
			keys = append(keys, key)
			fields[key] = field
		}
		sort.Strings(keys)
		children := []string{"map"}
		for _, key := range keys {
			_, id, err := o.walkNative(fields[key], childPath(key), key)
			if err != nil {
				return "", "", err
			}
			children = append(children, key, id)
		}
		identity = digest(children...)
	default:
		return "", "", fmt.Errorf("unsupported catalogue native type %v", v.Type())
	}
	if len(path) > 1 || path[0] == "body" {
		o.Entries = append(o.Entries, Entry{Path: path, Kind: kind, NativeText: &text, NativeIdentity: identity, Container: kind == "map" || kind == "array", Attribute: len(path) > 1, KeyText: key})
	}
	return text, identity, nil
}
