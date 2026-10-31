package searchindex

import (
	"context"
	"fmt"
)

type TextValue struct {
	Path []any
	Text string
}
type Result struct {
	DatasetID, ExperimentID, Name, Evaluation, Source string
	Ordinal                                           int64
	Values                                            []TextValue
}

func IndexResults(ctx context.Context, exec Executor, rows []Result) error {
	var objects []Object
	for _, r := range rows {
		parent := Object{Ref: "experiment:experiment:" + r.ExperimentID, Signal: "experiment", Owner: "experiment", RootKey: r.ExperimentID, RecordKey: r.ExperimentID}
		if err := parent.AddStrings(map[string]any{"name": r.Name, "evaluation": r.Evaluation, "source": r.Source}); err != nil {
			return err
		}
		child := Object{Ref: fmt.Sprintf("experiment:result:%s:%d", r.DatasetID, r.Ordinal), Signal: "experiment", Owner: "result", RootKey: r.ExperimentID, RecordKey: fmt.Sprint(r.Ordinal), ParentRef: parent.Ref}
		for _, v := range r.Values {
			text := v.Text
			child.Entries = append(child.Entries, Entry{Path: v.Path, Kind: "string", NativeText: &text})
		}
		objects = append(objects, parent, child)
	}
	return Write(ctx, exec, objects)
}
