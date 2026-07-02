// Package warehouse resolves customer orders against a catalog and produces
// shipments: Catalog (catalog.go) -> Resolve (orders.go) -> Fill (pick.go).
package warehouse

// Item is one stock-keeping unit.
type Item struct {
	SKU        string
	Name       string
	Stock      int
	Backordered bool // temporarily out of stock; still orderable
}

// Pick is a resolved order line: a position in the catalog's item slice.
type Pick struct {
	Pos int
	Qty int
}

// Shipment is what Fill produces for one order.
type Shipment struct {
	SKUs    []string
	Partial bool // some quantity is on backorder
}
