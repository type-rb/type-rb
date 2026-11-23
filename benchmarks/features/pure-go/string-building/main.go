// String interpolation, concatenation, conversion and joining.
package main

import (
	"fmt"
	"strconv"
	"strings"
)

func build(count int64) int64 {
	var total int64
	for index := int64(0); index < count; index++ {
		label := "item-" + strconv.FormatInt(index, 10) + ":" + strconv.FormatInt(index%97, 10)
		parts := []string{label, "x", label}
		total += int64(len(strings.Join(parts, "/")))
	}
	return total
}

func main() {
	fmt.Println(build(600000))
}
