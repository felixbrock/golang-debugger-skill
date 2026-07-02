package warehouse

// Fill walks the picks and produces a shipment. Quantity beyond available
// stock is marked as partial (backorder) rather than failing the order.
func Fill(c *Catalog, picks []Pick) Shipment {
	var s Shipment
	for _, p := range picks {
		it := c.At(p.Pos)
		s.SKUs = append(s.SKUs, it.SKU)
		if it.Stock < p.Qty {
			s.Partial = true
		}
	}
	return s
}
