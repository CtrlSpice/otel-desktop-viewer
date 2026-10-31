package queries

import (
	"fmt"
	"slices"
	"testing"
)

func TestMinimumMetricLabelColumns(t *testing.T) {
	columns := []metricLabelColumn{
		{Key: "model", Groups: []int{0, 0, 1, 1}, Cost: 10},
		{Key: "session.id", Groups: []int{4, 4, 8, 8}, Cost: 100},
		{Key: "type", Groups: []int{0, 1, 0, 1}, Cost: 10},
		{Key: "constant", Groups: []int{0, 0, 0, 0}, Cost: 1},
	}
	selected := minimumMetricLabelColumns(slices.Clone(columns))
	if len(selected) != 2 || selected[0].Key != "type" || selected[1].Key != "model" {
		t.Fatalf("selected = %+v", selected)
	}
	slices.Reverse(columns)
	if got := minimumMetricLabelColumns(columns); !slices.EqualFunc(got, selected, func(a, b metricLabelColumn) bool { return a.Key == b.Key }) {
		t.Fatalf("selection changed with column order: %+v", got)
	}
}

func TestMinimumMetricLabelColumnsGreedyPreservesPartition(t *testing.T) {
	var columns []metricLabelColumn
	for mask := 1; mask < 128; mask++ {
		column := metricLabelColumn{Key: fmt.Sprintf("parity-%03d", mask), Cost: 1}
		for row := range 128 {
			x := row & mask
			parity := 0
			for x != 0 {
				parity ^= x & 1
				x >>= 1
			}
			column.Groups = append(column.Groups, parity)
		}
		columns = append(columns, column)
	}
	selected := minimumMetricLabelColumns(columns)
	initial := metricLabelPartition{make([]int, 128)}
	for i := range 128 {
		initial[0][i] = i
	}
	if len(selected) > 7 || len(refineMetricLabels(initial, selected)) != 128 {
		t.Fatalf("greedy selected %d columns without preserving all distinctions", len(selected))
	}
}

func TestMinimumMetricLabelColumnsFindsMinimumForSmallCohorts(t *testing.T) {
	// Exhaustively compare the bounded selector with every subset for a small
	// deterministic collection of non-equivalent candidate partitions.
	for seed := range 64 {
		var columns []metricLabelColumn
		for c := range 6 {
			column := metricLabelColumn{Key: fmt.Sprint(c), Cost: int64(c)}
			for row := range 8 {
				column.Groups = append(column.Groups, ((row+seed)*(c+3)^(row>>(c%3)))%3)
			}
			columns = append(columns, column)
		}
		initial := metricLabelPartition{[]int{0, 1, 2, 3, 4, 5, 6, 7}}
		target := len(refineMetricLabels(initial, columns))
		minimum := len(columns)
		for mask := range 1 << len(columns) {
			var subset []metricLabelColumn
			for i, column := range columns {
				if mask&(1<<i) != 0 {
					subset = append(subset, column)
				}
			}
			if len(refineMetricLabels(initial, subset)) == target {
				minimum = min(minimum, len(subset))
			}
		}
		selected := minimumMetricLabelColumns(slices.Clone(columns))
		if len(selected) != minimum || len(refineMetricLabels(initial, selected)) != target {
			t.Fatalf("seed %d: selected %d, minimum %d", seed, len(selected), minimum)
		}
	}
}
