// Integer-keyed Hash insertion, update, membership and lookup.
package main

import "fmt"

func run(count int64) int64 {
	table := map[int64]int64{}
	for index := int64(0); index < count; index++ {
		key := (index * 7919) % 65536
		if value, ok := table[key]; ok {
			table[key] = value + index%13
		} else {
			table[key] = index % 13
		}
	}
	var total int64
	for probe := int64(0); probe < 65536; probe++ {
		if value, ok := table[probe]; ok {
			total += value
		}
	}
	return total + int64(len(table))
}

func main() {
	fmt.Println(run(2400000))
}
