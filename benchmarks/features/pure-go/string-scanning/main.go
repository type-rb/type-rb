// UTF-8 String splitting, code-point sizes and predicates.
package main

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

func scan(rounds int64) int64 {
	line := "alpha,βeta,gamma,δelta,epsilon,ζeta,eta,θeta,iota,κappa"
	var total int64
	for round := int64(0); round < rounds; round++ {
		parts := strings.Split(line, ",")
		for _, part := range parts {
			total += int64(utf8.RuneCountInString(part))
			if strings.HasPrefix(part, "e") {
				total += 3
			}
			if strings.Contains(part, "ta") {
				total++
			}
		}
		total += int64(len(parts))
	}
	return total
}

func main() {
	fmt.Println(scan(210000))
}
