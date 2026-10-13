package main

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Check the decoded fields before section rendering can conceal absent/null
// members. JSON output remains the unmodified RPC result.
func checkDetailFields(value any, fields map[string]func(any) bool) error {
	object, ok := value.(map[string]any)
	if !ok || object == nil {
		return fmt.Errorf("expected a detail object")
	}
	for name, valid := range fields {
		field, present := object[name]
		if !present {
			return fmt.Errorf("missing detail field %s", name)
		}
		if !valid(field) {
			return fmt.Errorf("invalid detail field %s", name)
		}
	}
	return nil
}

func detailString(value any) bool { _, ok := value.(string); return ok }
func detailBool(value any) bool   { _, ok := value.(bool); return ok }

func detailUintText(value any) bool {
	text, ok := value.(string)
	if !ok || text == "" || strings.HasPrefix(text, "+") {
		return false
	}
	_, err := strconv.ParseUint(text, 10, 64)
	return err == nil
}

func detailIntText(value any) bool {
	text, ok := value.(string)
	if !ok || text == "" || strings.HasPrefix(text, "+") {
		return false
	}
	_, err := strconv.ParseInt(text, 10, 64)
	return err == nil
}

func detailUintNumber(value any) bool {
	number, ok := value.(json.Number)
	if !ok {
		return false
	}
	_, err := strconv.ParseUint(string(number), 10, 32)
	return err == nil
}

func detailIntNumber(value any) bool {
	number, ok := value.(json.Number)
	if !ok {
		return false
	}
	_, err := strconv.ParseInt(string(number), 10, 32)
	return err == nil
}

func detailDouble(value any) bool {
	if number, ok := value.(json.Number); ok {
		parsed, err := number.Float64()
		return err == nil && !math.IsInf(parsed, 0) && !math.IsNaN(parsed)
	}
	bits, ok := value.(string)
	if !ok || len(bits) != 18 || !strings.HasPrefix(bits, "0x") {
		return false
	}
	_, err := hex.DecodeString(bits[2:])
	return err == nil
}

func detailRef(value any) bool {
	text, ok := value.(string)
	if !ok {
		return false
	}
	_, err := normalizeDetailRef(text, "record")
	return err == nil
}

func detailTraceID(value any) bool {
	if value == nil {
		return true
	}
	text, ok := value.(string)
	if !ok || len(text) != 32 {
		return false
	}
	_, err := hex.DecodeString(text)
	return err == nil
}

func detailSpanID(value any) bool {
	if value == nil {
		return true
	}
	text, ok := value.(string)
	if !ok {
		return false
	}
	_, err := normalizeSpanID(text)
	return err == nil
}

func detailArray(value any, valid func(any) bool) bool {
	items, ok := value.([]any)
	if !ok || items == nil {
		return false
	}
	for _, item := range items {
		if !valid(item) {
			return false
		}
	}
	return true
}

func detailAttributes(value any) bool {
	return detailArray(value, func(entry any) bool {
		return checkDetailFields(entry, map[string]func(any) bool{
			"key": detailString, "value": detailValue,
		}) == nil
	})
}

func detailValue(value any) bool {
	object, ok := value.(map[string]any)
	if !ok {
		return false
	}
	payload, present := object["value"]
	if !present {
		return false
	}
	switch object["kind"] {
	case "empty":
		return payload == nil
	case "string":
		return detailString(payload)
	case "bool":
		return detailBool(payload)
	case "int64":
		return detailIntText(payload)
	case "double":
		return detailDouble(payload)
	case "bytes":
		text, ok := payload.(string)
		if !ok {
			return false
		}
		_, err := base64.StdEncoding.DecodeString(text)
		return err == nil
	case "array":
		return detailArray(payload, detailValue)
	case "map":
		return detailAttributes(payload)
	default:
		return false
	}
}

func detailResourceFields(value any) bool {
	return checkDetailFields(value, map[string]func(any) bool{
		"attributes": detailAttributes, "droppedAttributesCount": detailUintNumber,
	}) == nil
}

func detailScopeFields(value any) bool {
	return detailResourceFields(value) && checkDetailFields(value, map[string]func(any) bool{
		"name": detailString, "version": detailString,
	}) == nil
}

func validateLogDetail(document map[string]any) error {
	return checkDetailFields(document, map[string]func(any) bool{
		"logRef": detailRef, "timestamp": detailUintText, "observedTimestamp": detailUintText,
		"traceID": detailTraceID, "spanID": detailSpanID,
		"severityText": detailString, "severityNumber": detailIntNumber,
		"body": detailValue, "flags": detailUintNumber, "eventName": detailString,
		"droppedAttributesCount": detailUintNumber, "attributes": detailAttributes,
		"resourceSchemaURL": detailString, "scopeSchemaURL": detailString,
		"resource": detailResourceFields, "scope": detailScopeFields,
	})
}

func detailNumberArms(value any) bool {
	object, ok := value.(map[string]any)
	if !ok {
		return false
	}
	integer, hasInt := object["intValue"]
	double, hasDouble := object["doubleValue"]
	if !hasInt || !hasDouble {
		return false
	}
	switch object["valueType"] {
	case "Int":
		return detailIntText(integer) && double == nil
	case "Double":
		return integer == nil && detailDouble(double)
	case "Empty":
		return integer == nil && double == nil
	default:
		return false
	}
}

func detailExemplar(value any) bool {
	return detailNumberArms(value) && checkDetailFields(value, map[string]func(any) bool{
		"timestamp": detailUintText, "traceID": detailTraceID, "spanID": detailSpanID,
		"filteredAttributes": detailAttributes,
	}) == nil
}

func detailBuckets(value any) bool {
	return checkDetailFields(value, map[string]func(any) bool{
		"offset":       detailIntNumber,
		"bucketCounts": func(value any) bool { return detailArray(value, detailUintText) },
	}) == nil
}

func detailDatapoint(value any, kind string) bool {
	if checkDetailFields(value, map[string]func(any) bool{
		"datapointRef": detailRef, "timestamp": detailUintText, "startTime": detailUintText,
		"flags":     detailUintNumber,
		"exemplars": func(value any) bool { return detailArray(value, detailExemplar) },
	}) != nil {
		return false
	}
	if kind == "Gauge" || kind == "Sum" {
		return detailNumberArms(value)
	}
	object := value.(map[string]any)
	for _, field := range []string{"sum", "min", "max"} {
		if value, present := object[field]; present && !detailDouble(value) {
			return false
		}
	}
	fields := map[string]func(any) bool{"count": detailUintText}
	if kind == "Histogram" {
		fields["bucketCounts"] = func(value any) bool { return detailArray(value, detailUintText) }
		fields["explicitBounds"] = func(value any) bool { return detailArray(value, detailDouble) }
	} else {
		fields["scale"] = detailIntNumber
		fields["zeroCount"] = detailUintText
		fields["zeroThreshold"] = detailDouble
		fields["positive"], fields["negative"] = detailBuckets, detailBuckets
	}
	return checkDetailFields(value, fields) == nil
}

func validateMetricDetail(document map[string]any, selected bool) error {
	if err := checkDetailFields(document, map[string]func(any) bool{
		"metricRef": detailRef, "name": detailString, "description": detailString,
		"unit": detailString, "metadata": detailAttributes,
		"resource": func(value any) bool {
			return detailResourceFields(value) && checkDetailFields(value, map[string]func(any) bool{"schemaUrl": detailString}) == nil
		},
		"scope": func(value any) bool {
			return detailScopeFields(value) && checkDetailFields(value, map[string]func(any) bool{"schemaUrl": detailString}) == nil
		},
	}); err != nil {
		return err
	}
	kind, ok := document["metricType"].(string)
	if !ok {
		return fmt.Errorf("missing or invalid metricType")
	}
	switch kind {
	case "Gauge":
	case "Sum", "Histogram", "ExponentialHistogram":
		fields := map[string]func(any) bool{"aggregationTemporalityCode": detailIntNumber}
		if kind == "Sum" {
			fields["isMonotonic"] = detailBool
		}
		if err := checkDetailFields(document, fields); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported metricType %q", kind)
	}
	if selected {
		return checkDetailFields(document, map[string]func(any) bool{
			"seriesRef": detailRef, "attributes": detailAttributes,
			"datapoints": func(value any) bool {
				return detailArray(value, func(point any) bool { return detailDatapoint(point, kind) })
			},
		})
	}
	return checkDetailFields(document, map[string]func(any) bool{
		"series": func(value any) bool {
			return detailArray(value, func(series any) bool {
				return checkDetailFields(series, map[string]func(any) bool{
					"seriesRef": detailRef, "attributes": detailAttributes, "datapointCount": detailUintText,
					"firstDatapointTimestamp": func(value any) bool { return value == nil || detailUintText(value) },
					"lastDatapointTimestamp":  func(value any) bool { return value == nil || detailUintText(value) },
				}) == nil
			})
		},
	})
}
