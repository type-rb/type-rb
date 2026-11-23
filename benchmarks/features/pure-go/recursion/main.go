// Recursive calls with small scalar arguments.
package main

import "fmt"

func fib(n int64) int64 {
	if n < 2 {
		return n
	}
	return fib(n-1) + fib(n-2)
}

func tak(x, y, z int64) int64 {
	if y < x {
		return tak(tak(x-1, y, z), tak(y-1, z, x), tak(z-1, x, y))
	}
	return z
}

func main() {
	fmt.Println(fib(35))
	fmt.Println(tak(24, 16, 8))
}
