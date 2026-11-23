// Class construction, field updates and interface dispatch.
package main

import "fmt"

type Meter interface {
	Measure(step int64) int64
}

type Linear struct {
	scale int64
	total int64
}

func (l *Linear) Measure(step int64) int64 {
	l.total = (l.total + step*l.scale) % 1000003
	return l.total
}

type Squared struct {
	offset int64
}

func (s *Squared) Measure(step int64) int64 {
	value := step%1000 + s.offset
	return value * value % 1000003
}

func run(rounds int64) int64 {
	meters := []Meter{&Linear{scale: 3}, &Squared{offset: 7}, &Linear{scale: 11}, &Squared{offset: 2}}
	var checksum int64
	for round := int64(0); round < rounds; round++ {
		meter := meters[round%int64(len(meters))]
		checksum = (checksum + meter.Measure(round)) % 1000000007
	}
	return checksum
}

func main() {
	fmt.Println(run(20000000))
}
