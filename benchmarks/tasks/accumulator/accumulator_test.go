package accumulator

import "testing"

func TestAveragesTheEvens(t *testing.T) {
	// evens of [2, 4, 6, 7, 9] are [2, 4, 6]; average 4; * 10 = 40.
	if got := ScaledEvenAverage([]int64{2, 4, 6, 7, 9}); got != 40 {
		t.Fatalf("ScaledEvenAverage = %d, want 40", got)
	}
}
