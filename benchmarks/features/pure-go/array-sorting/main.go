// Array construction, natural sorting and stable key sorting.
package main

import (
	"fmt"
	"slices"
	"sort"
)

func run(count int, rounds int) int64 {
	seed := int64(42)
	var checksum int64
	for round := 0; round < rounds; round++ {
		values := make([]int64, 0)
		for index := 0; index < count; index++ {
			seed = (seed*1664525 + 1013904223) % 4294967296
			values = append(values, seed%100000)
		}
		sorted := slices.Clone(values)
		slices.Sort(sorted)
		checksum = (checksum + sorted[0] + sorted[count/2] + sorted[count-1]) % 1000000007
		bySuffix := slices.Clone(values)
		sort.SliceStable(bySuffix, func(i, j int) bool { return bySuffix[i]%1000 < bySuffix[j]%1000 })
		checksum = (checksum + bySuffix[0] + bySuffix[count-1]) % 1000000007
	}
	return checksum
}

func main() {
	fmt.Println(run(20000, 24))
}
