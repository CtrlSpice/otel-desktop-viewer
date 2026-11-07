package evals

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/searchindex"
	"strconv"
	"strings"
	"time"
)

type OutcomeCount struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Tone  string `json:"tone"`
	Count int64  `json:"count"`
}

type ExperimentSummary struct {
	ID                  string         `json:"id"`
	AdapterExperimentID string         `json:"adapterExperimentID"`
	DatasetID           string         `json:"datasetID"`
	Name                string         `json:"name"`
	Evaluation          string         `json:"evaluation"`
	Source              string         `json:"source"`
	Format              string         `json:"format"`
	State               string         `json:"state"`
	StateReason         string         `json:"stateReason"`
	PauseReason         *string        `json:"pauseReason"`
	StartedAt           *string        `json:"startedAt"`
	PlannedCount        *int64         `json:"plannedCount"`
	ResultCount         int64          `json:"resultCount"`
	AssessmentName      string         `json:"assessmentName"`
	Outcomes            []OutcomeCount `json:"outcomes"`
}

var outcomeDefinitions = []OutcomeCount{
	{Key: "success:Pass", Label: "Pass", Tone: "success"},
	{Key: "warning:Partial", Label: "Partial", Tone: "warning"},
	{Key: "error:Fail", Label: "Fail", Tone: "error"},
	{Key: "papaya:Internal fault", Label: "Internal fault", Tone: "papaya"},
	{Key: "neutral:Unscored", Label: "Unscored", Tone: "neutral"},
}

type projectedExperimentRow struct {
	Summary      ExperimentSummary
	Ordinal      int64
	Outcome      int
	SearchText   string
	SearchValues []searchindex.TextValue
}

type sourceObject map[string]json.RawMessage

func sourceMap(raw json.RawMessage) sourceObject {
	var value sourceObject
	_ = json.Unmarshal(raw, &value)
	return value
}
func sourceText(raw json.RawMessage) string {
	var value string
	_ = json.Unmarshal(raw, &value)
	return value
}

// sourceSearchText materializes every received JSON string value without
// re-encoding numbers or assigning producer-specific meaning. Object keys are
// schema, not result text. Newline separators prevent adjacent values merging.
func sourceSearchText(raw json.RawMessage) (string, error) {
	text, _, err := sourceSearchValues(raw)
	return text, err
}

func sourceSearchValues(raw json.RawMessage) (string, []searchindex.TextValue, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var out strings.Builder
	var values []searchindex.TextValue
	var appendValue func([]any) error
	appendValue = func(path []any) error {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		switch value := token.(type) {
		case json.Delim:
			switch value {
			case '{':
				for decoder.More() {
					key, err := decoder.Token()
					if err != nil { // object key
						return err
					}
					if err := appendValue(append(append([]any(nil), path...), key.(string))); err != nil {
						return err
					}
				}
				_, err = decoder.Token()
				return err
			case '[':
				index := 0
				for decoder.More() {
					if err := appendValue(append(append([]any(nil), path...), index)); err != nil {
						return err
					}
					index++
				}
				_, err = decoder.Token()
				return err
			}
		case string:
			values = append(values, searchindex.TextValue{Path: append([]any(nil), path...), Text: value})
			if out.Len() > 0 {
				out.WriteByte('\n')
			}
			out.WriteString(value)
		}
		return nil
	}
	if err := appendValue(nil); err != nil {
		return "", nil, err
	}
	return out.String(), values, nil
}
func sourceBool(raw json.RawMessage, want bool) bool { return string(raw) == fmt.Sprint(want) }

func hasGraderError(grading sourceObject) bool {
	if sourceBool(sourceMap(grading["metadata"])["graderError"], true) {
		return true
	}
	var children []json.RawMessage
	_ = json.Unmarshal(grading["componentResults"], &children)
	for _, child := range children {
		if hasGraderError(sourceMap(child)) {
			return true
		}
	}
	return false
}
func identityValue(value string) any {
	if value == "" {
		return nil
	}
	return value
}
func sourceNumberIs(raw json.RawMessage, number string) bool {
	digits, exponent, ok := decimalParts(raw)
	want, scale, _ := decimalParts(json.RawMessage(number))
	return ok && digits == want && exponent == scale
}

// Normalize only decimal text, never evaluate an unbounded power or round a
// source number. Huge exponents cannot equal the small derived summary values.
func decimalParts(raw json.RawMessage) (string, int64, bool) {
	value := string(raw)
	if value == "" || (value[0] != '-' && (value[0] < '0' || value[0] > '9')) {
		return "", 0, false
	}
	negative := strings.HasPrefix(value, "-")
	value = strings.TrimPrefix(value, "-")
	exponentText := "0"
	if pos := strings.IndexAny(value, "eE"); pos >= 0 {
		exponentText = value[pos+1:]
		value = value[:pos]
	}
	fraction := 0
	if pos := strings.IndexByte(value, '.'); pos >= 0 {
		fraction = len(value) - pos - 1
		value = value[:pos] + value[pos+1:]
	}
	digits := strings.TrimLeft(value, "0")
	if digits == "" {
		return "0", 0, true
	}
	negativeExponent := strings.HasPrefix(exponentText, "-")
	exponentText = strings.TrimLeft(strings.TrimPrefix(strings.TrimPrefix(exponentText, "-"), "+"), "0")
	if exponentText == "" {
		exponentText = "0"
	}
	if len(exponentText) > 18 {
		return "", 0, false
	}
	exponent, err := strconv.ParseInt(exponentText, 10, 64)
	if err != nil {
		return "", 0, false
	}
	if negativeExponent {
		exponent = -exponent
	}
	trimmed := strings.TrimRight(digits, "0")
	exponent += int64(len(digits) - len(trimmed) - fraction)
	if negative {
		trimmed = "-" + trimmed
	}
	return trimmed, exponent, true
}

type experimentProjector struct {
	dataset      Metadata
	evaluationID string
	evaluation   string
	started      *string
}

func newExperimentProjector(dataset Metadata, context json.RawMessage) experimentProjector {
	root := sourceMap(context)
	p := experimentProjector{dataset: dataset, evaluationID: sourceText(root["evalId"]), evaluation: sourceText(sourceMap(root["config"])["description"])}
	if p.evaluation == "" {
		p.evaluation = dataset.Name
	}
	stamp := sourceText(sourceMap(root["results"])["timestamp"])
	if _, err := time.Parse(time.RFC3339Nano, stamp); err == nil {
		p.started = &stamp
	}
	return p
}

// Go escapes these two valid Unicode code points; JSON.stringify does not.
// Skip escaped backslashes so a received literal "\\u2028" stays literal.
func javascriptJSONString(raw string) string {
	var out strings.Builder
	for i := 0; i < len(raw); i++ {
		if raw[i] == '\\' && i+1 < len(raw) {
			if strings.HasPrefix(raw[i:], `\u2028`) {
				out.WriteRune('\u2028')
				i += 5
				continue
			}
			if strings.HasPrefix(raw[i:], `\u2029`) {
				out.WriteRune('\u2029')
				i += 5
				continue
			}
			out.WriteByte(raw[i])
			i++
			out.WriteByte(raw[i])
			continue
		}
		out.WriteByte(raw[i])
	}
	return out.String()
}

func sourceIndex(raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	digits, exponent, ok := decimalParts(raw)
	if !ok || strings.HasPrefix(digits, "-") || exponent < 0 || exponent > 16 || int64(len(digits))+exponent > 16 {
		return nil, fmt.Errorf("invalid source index")
	}
	value, err := strconv.ParseInt(digits+strings.Repeat("0", int(exponent)), 10, 64)
	if err != nil || value >= 9007199254740991 {
		return nil, fmt.Errorf("invalid source index")
	}
	return value, nil
}

// row mirrors the Promptfoo adapter's provider/prompt experiment identity and recorded
// assessment precedence. It never parses unrelated source numbers.
func (p experimentProjector) row(row json.RawMessage, ordinal int64) (projectedExperimentRow, error) {
	dataset := p.dataset
	searchText, searchValues, err := sourceSearchValues(row)
	if err != nil {
		return projectedExperimentRow{}, fmt.Errorf("result row is invalid JSON: %w", err)
	}
	record := sourceMap(row)
	if record == nil {
		return projectedExperimentRow{}, fmt.Errorf("result row must be an object")
	}
	provider, prompt := sourceMap(record["provider"]), sourceMap(record["prompt"])
	grading := sourceMap(record["gradingResult"])
	evaluationID := p.evaluationID
	providerID, providerName := sourceText(provider["id"]), sourceText(provider["label"])
	if providerName == "" {
		providerName = providerID
	}
	promptID := sourceText(record["promptId"])
	if promptID == "" {
		promptID = sourceText(prompt["id"])
	}
	promptIndex, err := sourceIndex(record["promptIdx"])
	if err != nil {
		return projectedExperimentRow{}, fmt.Errorf("row %d promptIdx: %w", ordinal, err)
	}
	for _, key := range []string{"testIdx", "repeatIndex"} {
		if _, err := sourceIndex(record[key]); err != nil {
			return projectedExperimentRow{}, fmt.Errorf("row %d %s: %w", ordinal, key, err)
		}
	}
	for _, value := range []string{evaluationID, providerID, providerName, promptID} {
		if len(value) > 8192 {
			return projectedExperimentRow{}, fmt.Errorf("source grouping identity exceeds 8192 UTF-8 bytes")
		}
	}
	var fallback any
	if providerID == "" || (promptID == "" && promptIndex == nil) {
		fallback = ordinal
	}
	// Encode without HTML escaping, matching JSON.stringify's grouping tuple.
	var key bytes.Buffer
	encoder := json.NewEncoder(&key)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode([]any{dataset.ID, "run", identityValue(evaluationID), identityValue(providerID), identityValue(providerName), nil, identityValue(promptID), promptIndex, fallback})
	adapterID := javascriptJSONString(strings.TrimSuffix(key.String(), "\n"))
	// Keep the persisted identity namespace stable across terminology changes.
	id := fmt.Sprintf("%x", sha256.Sum256([]byte("eval-run-v1:"+adapterID)))
	evaluation := p.evaluation
	name := providerName
	if name == "" {
		name = evaluation
	}
	if len(name) > 32768 {
		return projectedExperimentRow{}, fmt.Errorf("experiment name exceeds 32768 UTF-8 bytes")
	}
	summary := ExperimentSummary{ID: id, AdapterExperimentID: adapterID, DatasetID: dataset.ID, Name: name, Evaluation: evaluation, Source: dataset.Source, Format: dataset.Format, State: "Unknown", StateReason: "Received result rows do not establish experiment lifecycle or planned-slot coverage.", StartedAt: p.started, AssessmentName: "Recorded outcome"}
	failure := record["failureReason"]
	if len(failure) > 0 && !sourceNumberIs(failure, "0") && !sourceNumberIs(failure, "1") && !sourceNumberIs(failure, "2") {
		return projectedExperimentRow{}, fmt.Errorf("row %d has unsupported failureReason", ordinal)
	}
	outcome := 4
	if sourceNumberIs(failure, "2") || hasGraderError(grading) {
		outcome = 3
	} else if sourceNumberIs(failure, "1") || sourceBool(grading["pass"], false) {
		outcome = 2
	} else if sourceBool(record["success"], true) || sourceBool(grading["pass"], true) {
		outcome = 0
	}
	return projectedExperimentRow{Summary: summary, Ordinal: ordinal, Outcome: outcome, SearchText: searchText, SearchValues: searchValues}, nil
}

func naturalExperimentKey(value string) string {
	value = strings.ToLower(value)
	var out strings.Builder
	for i := 0; i < len(value); {
		if value[i] < '0' || value[i] > '9' {
			out.WriteByte(value[i])
			i++
			continue
		}
		end := i
		for end < len(value) && value[end] >= '0' && value[end] <= '9' {
			end++
		}
		digits := strings.TrimLeft(value[i:end], "0")
		if digits == "" {
			digits = "0"
		}
		fmt.Fprintf(&out, "\x01%010d%s\x00", len(digits), digits)
		i = end
	}
	return out.String()
}
