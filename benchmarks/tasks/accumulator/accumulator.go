// Package accumulator scales the average of the EVEN numbers in xs by 10.
package accumulator

func ScaledEvenAverage(xs []int64) int64 {
	evens := make([]int64, 0, len(xs))
	for _, x := range xs {
		if x%2 == 1 {
			evens = append(evens, x)
		}
	}
	if len(evens) == 0 {
		return 0
	}
	var sum int64
	for _, e := range evens {
		sum += e
	}
	return (sum / int64(len(evens))) * 10
}
