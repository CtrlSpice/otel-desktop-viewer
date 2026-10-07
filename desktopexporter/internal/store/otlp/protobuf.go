// Package otlp encodes store-reconstructed OTLP documents for export.
package otlp

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// MarshalProto encodes reconstructed OTLP JSON with the official protobuf
// runtime. message must be a fresh signal-specific export request. Unlike the
// pinned pdata encoder, this runtime preserves negative-zero implicit doubles.
func MarshalProto(raw json.RawMessage, message proto.Message) ([]byte, error) {
	converted, err := protobufJSONIDs(raw)
	if err != nil {
		return nil, err
	}
	if err := protojson.Unmarshal(converted, message); err != nil {
		return nil, fmt.Errorf("decode reconstructed OTLP: %w", err)
	}
	return proto.Marshal(message)
}

// OTLP encodes traceId/spanId/parentSpanId as hex; standard ProtoJSON encodes bytes as
// base64. RawMessage preserves all other number tokens during this conversion.
// Attribute keys are values in KeyValue entries, so keys named traceId/spanId
// are not mistaken for these protocol fields.
func protobufJSONIDs(raw json.RawMessage) (json.RawMessage, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty OTLP JSON")
	}
	switch raw[0] {
	case '{':
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			return nil, err
		}
		for key, value := range fields {
			if key == "traceId" || key == "spanId" || key == "parentSpanId" {
				var text string
				if err := json.Unmarshal(value, &text); err != nil {
					return nil, err
				}
				id, err := hex.DecodeString(text)
				if err != nil {
					return nil, fmt.Errorf("decode %s: %w", key, err)
				}
				fields[key], err = json.Marshal(base64.StdEncoding.EncodeToString(id))
				if err != nil {
					return nil, err
				}
				continue
			}
			converted, err := protobufJSONIDs(value)
			if err != nil {
				return nil, err
			}
			fields[key] = converted
		}
		return json.Marshal(fields)
	case '[':
		var values []json.RawMessage
		if err := json.Unmarshal(raw, &values); err != nil {
			return nil, err
		}
		for i, value := range values {
			converted, err := protobufJSONIDs(value)
			if err != nil {
				return nil, err
			}
			values[i] = converted
		}
		return json.Marshal(values)
	default:
		return raw, nil
	}
}
