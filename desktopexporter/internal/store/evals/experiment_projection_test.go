package evals

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExperimentProjectionIdentityAndAssessmentSemantics(t *testing.T) {
	metadata := Metadata{ID: "dataset", Name: "source.json", Source: "json-import", Format: "promptfoo-v3"}
	projector := newExperimentProjector(metadata, json.RawMessage(`{"evalId":"evaluation","config":{"description":"Question set"},"results":{"version":3,"timestamp":"2026-10-21T12:00:00Z","results":[]}}`))
	row := json.RawMessage(`{"provider":{"id":"p","label":"Provider"},"promptId":"prompt","promptIdx":2,"prompt":{"label":"Prompt"},"success":true,"future":9007199254740993}`)
	first, err := projector.row(row, 7)
	require.NoError(t, err)
	second, err := projector.row(row, 99)
	require.NoError(t, err)
	require.Equal(t, first.Summary.ID, second.Summary.ID)
	require.Equal(t, `["dataset","run","evaluation","p","Provider",null,"prompt",2,null]`, first.Summary.AdapterExperimentID)
	require.Equal(t, "Provider", first.Summary.Name)
	require.Equal(t, "Unknown", first.Summary.State)
	require.Nil(t, first.Summary.PlannedCount)
	require.Equal(t, 0, first.Outcome)
	for _, input := range []struct {
		json    string
		outcome int
	}{
		{`{"success":true,"gradingResult":{"pass":false}}`, 2},
		{`{"success":true,"failureReason":2}`, 3},
		{`{"success":false,"failureReason":1,"gradingResult":{"componentResults":[{"pass":false,"metadata":{"graderError":true}}]}}`, 3},
		{`{"gradingResult":{"score":1}}`, 4}, // Promptfoo numeric maximum is unknown.
		{`{"success":false}`, 4},             // false alone does not prove a failed assessment.
	} {
		p, err := projector.row(json.RawMessage(input.json), 1)
		require.NoError(t, err)
		require.Equal(t, input.outcome, p.Outcome)
	}
	one, err := projector.row(json.RawMessage(`{}`), 1)
	require.NoError(t, err)
	two, err := projector.row(json.RawMessage(`{}`), 2)
	require.NoError(t, err)
	require.NotEqual(t, one.Summary.ID, two.Summary.ID)
	unicode, err := projector.row(json.RawMessage(`{"provider":{"id":"雪\u2028"},"promptId":"\\u2028"}`), 0)
	require.NoError(t, err)
	require.Contains(t, unicode.Summary.AdapterExperimentID, "雪\u2028")
	require.Contains(t, unicode.Summary.AdapterExperimentID, `"\\u2028"`)
	require.True(t, strings.Compare(naturalExperimentKey("Experiment 2"), naturalExperimentKey("Experiment 10")) < 0)
}
