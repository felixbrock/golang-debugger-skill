package warehouse

import "fmt"

// OrderLine is one requested SKU and quantity.
type OrderLine struct {
	SKU string
	Qty int
}

// Resolve maps an order's lines to catalog positions, merging duplicate
// lines for the same SKU. Unknown SKUs are an error.
func Resolve(c *Catalog, lines []OrderLine) ([]Pick, error) {
	byPos := map[int]int{}
	var order []int
	for _, ln := range lines {
		if ln.Qty <= 0 {
			return nil, fmt.Errorf("bad quantity %d for %s", ln.Qty, ln.SKU)
		}
		pos := c.Lookup(ln.SKU)
		if pos < 0 {
			return nil, fmt.Errorf("unknown SKU %q", ln.SKU)
		}
		if _, seen := byPos[pos]; !seen {
			order = append(order, pos)
		}
		byPos[pos] += ln.Qty
	}
	picks := make([]Pick, 0, len(order))
	for _, pos := range order {
		picks = append(picks, Pick{Pos: pos, Qty: byPos[pos]})
	}
	return picks, nil
}
