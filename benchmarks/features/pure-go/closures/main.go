// Function values, captured state and block-based collection operations.
package main

import "fmt"

func apply(values []int64, transform func(int64) int64) int64 {
	var total int64
	for _, value := range values {
		total += transform(value)
	}
	return total
}

func run(rounds int64) int64 {
	values := make([]int64, 0)
	for index := int64(0); index < 200; index++ {
		values = append(values, index)
	}
	var checksum int64
	for round := int64(0); round < rounds; round++ {
		offset := round % 17
		shifted := func(value int64) int64 { return value + offset }
		checksum += apply(values, shifted)
		evens := make([]int64, 0)
		for _, value := range values {
			if (value+offset)%2 == 0 {
				evens = append(evens, value)
			}
		}
		tripled := make([]int64, 0, len(evens))
		for _, value := range evens {
			tripled = append(tripled, value*3)
		}
		var sum int64
		for _, value := range tripled {
			sum += value
		}
		checksum += sum
		checksum = checksum % 1000000007
	}
	return checksum
}

func main() {
	fmt.Println(run(60000))
}
