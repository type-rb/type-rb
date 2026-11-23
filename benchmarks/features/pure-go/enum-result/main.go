// Enum payload matching and Result propagation.
package main

import (
	"errors"
	"fmt"
)

type Command struct {
	kind  int
	value int64
}

const (
	add = iota
	scale
	reset
)

func command(index int64) Command {
	switch index % 9 {
	case 0:
		return Command{kind: scale, value: index%3 + 2}
	case 8:
		return Command{kind: reset}
	}
	return Command{kind: add, value: index % 11}
}

func checked(value int64) (int64, error) {
	if value > 40 {
		return 0, errors.New("overflow")
	}
	return value, nil
}

func step(value int64, next Command) (int64, error) {
	switch next.kind {
	case add:
		return checked(value + next.value)
	case scale:
		scaled, err := checked(value * next.value)
		if err != nil {
			return 0, err
		}
		return scaled % 9973, nil
	default:
		return 1, nil
	}
}

func run(rounds int64) int64 {
	value := int64(1)
	var failures, checksum int64
	for index := int64(0); index < rounds; index++ {
		next, err := step(value, command(index))
		if err != nil {
			failures++
			value = index%7 + 1
		} else {
			value = next
		}
		checksum = (checksum + value) % 1000000007
	}
	fmt.Println(failures)
	return checksum
}

func main() {
	fmt.Println(run(8000000))
}
