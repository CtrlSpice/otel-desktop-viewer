package logs

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"log"

	"github.com/marcboeker/go-duckdb/v2"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/errors"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/util"
)

// Body supports all value types according to semantic conventions:
// - Scalar values: string, boolean, signed 64-bit integer, double
// - Byte array
// - Everything else (arrays, maps, etc.) as JSON
type Body struct {
	Data any
}

// MarshalJSON implements json.Marshaler to serialize the body as just the data value
// This is because our wrapper for duckdb.Union has no business leaking to the frontend.
func (body Body) MarshalJSON() ([]byte, error) {
	return json.Marshal(body.Data)
}

// UnmarshalJSON implements json.Unmarshaler to deserialize the body from JSON
func (body *Body) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &body.Data)
}

// Value converts a log body value to a DuckDB Union type.
// For uint64 values, if they exceed math.MaxInt64, they are converted to strings.
// For complex types (arrays, maps, structs), the value is JSON marshaled.
func (body Body) Value() (driver.Value, error) {
	switch t := body.Data.(type) {
	case string:
		return duckdb.Union{Tag: "str", Value: t}, nil
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32:
		return duckdb.Union{Tag: "bigint", Value: t}, nil
	case uint64:
		value, hasOverflow := util.StringifyOnOverflow("body", t)
		if hasOverflow {
			return duckdb.Union{Tag: "str", Value: value}, nil
		}
		return duckdb.Union{Tag: "bigint", Value: value}, nil
	case float32, float64:
		return duckdb.Union{Tag: "double", Value: t}, nil
	case bool:
		return duckdb.Union{Tag: "boolean", Value: t}, nil
	case []byte:
		return duckdb.Union{Tag: "bytes", Value: t}, nil
	default:
		// For complex types (arrays, maps, structs), convert to JSON string
		bodyJson, err := json.Marshal(body)
		if err != nil {
			log.Printf(errors.WarnJSONMarshal, t, body.Data)
			return duckdb.Union{Tag: "str", Value: fmt.Sprintf("%v", body)}, nil
		}
		return duckdb.Union{Tag: "json", Value: string(bodyJson)}, nil
	}
}

func (body *Body) Scan(src any) error {
	switch v := src.(type) {
	case duckdb.Union:
		if v.Tag == "json" {
			strValue, ok := v.Value.(string)
			if !ok {
				log.Printf(errors.WarnJSONUnmarshal, fmt.Sprintf(errors.ErrJSONValueType, v.Value))
				body.Data = v.Value
				return nil
			}

			var result any
			if err := json.Unmarshal([]byte(strValue), &result); err != nil {
				log.Printf(errors.WarnJSONUnmarshal, err)
				body.Data = v.Value
				return nil
			}
			body.Data = result
			return nil
		}
		body.Data = v.Value
		return nil
	case map[string]interface{}:
		// Handle case where AppenderWrapper converted duckdb.Union to map[string]interface{}
		if tag, hasTag := v["tag"].(string); hasTag {
			if value, hasValue := v["value"]; hasValue {
				if tag == "json" {
					strValue, ok := value.(string)
					if !ok {
						log.Printf(errors.WarnJSONUnmarshal, fmt.Sprintf(errors.ErrJSONValueType, value))
						body.Data = value
						return nil
					}

					var result any
					if err := json.Unmarshal([]byte(strValue), &result); err != nil {
						log.Printf(errors.WarnJSONUnmarshal, err)
						body.Data = value
						return nil
					}
					body.Data = result
					return nil
				}
				body.Data = value
				return nil
			}
		}
		// Fallback: treat the whole map as the body data
		body.Data = v
		return nil
	case nil:
		body.Data = nil
		return nil
	default:
		return fmt.Errorf("Body: cannot scan from %T", src)
	}
}
