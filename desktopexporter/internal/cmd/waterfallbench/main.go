//go:build waterfallbench

package main

import "fmt"

const benchmarkSentinel = "__WATERFALL_BENCHMARK__"

func main() {
	fmt.Println(benchmarkSentinel)
}
