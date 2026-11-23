// String-keyed Hash counting with generated keys.
package main

import (
	"fmt"
	"strconv"
)

func run(rounds int64) int64 {
	words := []string{"red", "green", "blue", "cyan", "magenta", "yellow", "black", "white"}
	counts := map[string]int64{}
	for round := int64(0); round < rounds; round++ {
		word := words[(round*5)%int64(len(words))] + strconv.FormatInt(round%50, 10)
		counts[word]++
	}
	var total int64
	for key, count := range counts {
		total += count * int64(len(key))
	}
	return total + int64(len(counts))
}

func main() {
	fmt.Println(run(1200000))
}
