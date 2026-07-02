package rpncalc

import "testing"

func TestEvaluatesExpressions(t *testing.T) {
	// "10 3 - 2 * 1 +"  ==  (10 - 3) * 2 + 1  ==  15
	if got := Calc("10 3 - 2 * 1 +"); got != 15 {
		t.Fatalf(`Calc("10 3 - 2 * 1 +") = %d, want 15`, got)
	}
	// "100 5 / 3 -"     ==  (100 / 5) - 3     ==  17
	if got := Calc("100 5 / 3 -"); got != 17 {
		t.Fatalf(`Calc("100 5 / 3 -") = %d, want 17`, got)
	}
}
