// Demo target for gdbg: total() has a bug — items with Qty == 0 reset the
// running sum instead of being skipped.
package main

import "fmt"

type Item struct {
	Name string
	Qty  int
}

func total(items []Item) int {
	sum := 0
	for _, it := range items {
		if it.Qty == 0 {
			sum = 0 // BUG: should be `continue`
		}
		sum += it.Qty
	}
	return sum
}

func main() {
	items := []Item{
		{Name: "apple", Qty: 3},
		{Name: "pear", Qty: 0},
		{Name: "plum", Qty: 7},
	}
	fmt.Println("total:", total(items)) // want 10, prints 7
}
