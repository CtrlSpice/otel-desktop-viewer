package queries

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/duckdb/duckdb-go/v2"
)

// SQL has already compared complete typed values and assigned exact equality
// classes. Column selection works only on these small integers, not telemetry.
type metricLabelColumn struct {
	Key    string `json:"key"`
	Groups []int  `json:"groups"`
	Cost   int64  `json:"cost"`
}

func sortMetricLabelKeys(keys []string) { slices.Sort(keys) }

func compareMetricLabelKeys(a, b string) int {
	return slices.Compare(utf16.Encode([]rune(a)), utf16.Encode([]rune(b)))
}

type metricLabelColumns struct{ config duckdb.ScalarFuncConfig }

func (f *metricLabelColumns) Config() duckdb.ScalarFuncConfig { return f.config }
func (f *metricLabelColumns) Executor() duckdb.ScalarFuncExecutor {
	return duckdb.ScalarFuncExecutor{RowExecutor: func(args []driver.Value) (any, error) {
		var columns []metricLabelColumn
		if err := json.Unmarshal([]byte(args[0].(string)), &columns); err != nil {
			return nil, fmt.Errorf("metric label columns: %w", err)
		}
		keysSeen := make(map[string]bool)
		for _, column := range columns {
			if len(column.Groups) != len(columns[0].Groups) || keysSeen[column.Key] {
				return nil, fmt.Errorf("metric label columns require unique keys and equal row counts")
			}
			keysSeen[column.Key] = true
		}
		selected := minimumMetricLabelColumns(columns)
		keys := make([]any, len(selected))
		for i, column := range selected {
			keys[i] = column.Key
		}
		return keys, nil
	}}
}

type metricLabelPartition [][]int

func splitMetricLabels(groups metricLabelPartition, column metricLabelColumn) metricLabelPartition {
	var result metricLabelPartition
	for _, group := range groups {
		positions := make(map[int]int)
		for _, row := range group {
			value := column.Groups[row]
			position, ok := positions[value]
			if !ok {
				position = len(result)
				positions[value] = position
				result = append(result, nil)
			}
			result[position] = append(result[position], row)
		}
	}
	return result
}

func refineMetricLabels(groups metricLabelPartition, columns []metricLabelColumn) metricLabelPartition {
	for _, column := range columns {
		groups = splitMetricLabels(groups, column)
	}
	return groups
}

// Keep the UI's bounded minimum-cover policy: at most 16 candidates and 100000
// row refinements for exact search; otherwise use deterministic greedy cover.
func minimumMetricLabelColumns(columns []metricLabelColumn) []metricLabelColumn {
	if len(columns) == 0 {
		return nil
	}
	for i := range columns {
		columns[i].Cost += int64(metricLabelLength(columns[i].Key) * len(columns[i].Groups))
	}
	slices.SortFunc(columns, func(a, b metricLabelColumn) int {
		if a.Cost < b.Cost {
			return -1
		}
		if a.Cost > b.Cost {
			return 1
		}
		return compareMetricLabelKeys(a.Key, b.Key)
	})
	seen := make(map[string]bool)
	columns = slices.DeleteFunc(columns, func(column metricLabelColumn) bool {
		var signature strings.Builder
		classes := make(map[int]int)
		for _, group := range column.Groups {
			class, ok := classes[group]
			if !ok {
				class = len(classes)
				classes[group] = class
			}
			signature.WriteString(strconv.Itoa(class))
			signature.WriteByte(',')
		}
		key := signature.String()
		if seen[key] {
			return true
		}
		seen[key] = true
		return false
	})
	rowCount := len(columns[0].Groups)
	initial := metricLabelPartition{make([]int, rowCount)}
	for i := range rowCount {
		initial[0][i] = i
	}
	target := len(refineMetricLabels(initial, columns))
	if target <= 1 {
		return nil
	}
	greedy := func() []metricLabelColumn {
		groups := initial
		var chosen []metricLabelColumn
		used := make(map[string]bool)
		for len(groups) < target {
			best := -1
			bestGroups := groups
			for i, column := range columns {
				if used[column.Key] {
					continue
				}
				refined := splitMetricLabels(groups, column)
				if len(refined) > len(bestGroups) {
					best = i
					bestGroups = refined
				}
			}
			if best < 0 {
				break
			}
			chosen = append(chosen, columns[best])
			used[columns[best].Key] = true
			groups = bestGroups
		}
		return chosen
	}
	if len(columns) > 16 {
		return greedy()
	}
	var required, remaining []metricLabelColumn
	for i, column := range columns {
		others := append(slices.Clone(columns[:i]), columns[i+1:]...)
		if len(refineMetricLabels(initial, others)) < target {
			required = append(required, column)
		} else {
			remaining = append(remaining, column)
		}
	}
	base := refineMetricLabels(initial, required)
	work := 100000
	refine := func(groups metricLabelPartition, column metricLabelColumn) metricLabelPartition {
		work -= rowCount
		return splitMetricLabels(groups, column)
	}
	var search func(int, int, metricLabelPartition) ([]metricLabelColumn, bool)
	search = func(start, slots int, groups metricLabelPartition) ([]metricLabelColumn, bool) {
		if len(groups) == target {
			return nil, true
		}
		if work <= 0 || slots == 0 || len(remaining)-start < slots {
			return nil, false
		}
		possible := groups
		for _, column := range remaining[start:] {
			possible = refine(possible, column)
		}
		if len(possible) < target {
			return nil, false
		}
		for i := start; i <= len(remaining)-slots; i++ {
			if work <= 0 {
				return nil, false
			}
			refined := refine(groups, remaining[i])
			if len(refined) == len(groups) {
				continue
			}
			if suffix, ok := search(i+1, slots-1, refined); ok {
				return append([]metricLabelColumn{remaining[i]}, suffix...), true
			}
		}
		return nil, false
	}
	for size := 0; size <= len(remaining); size++ {
		if chosen, ok := search(0, size, base); ok {
			return append(required, chosen...)
		}
		if work <= 0 {
			return greedy()
		}
	}
	return columns
}
