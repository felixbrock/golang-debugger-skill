package warehouse

import "strings"

// Catalog stores items and an index from normalized SKU to the item's
// position in items.
type Catalog struct {
	items []Item
	index map[string]int
}

func NewCatalog() *Catalog {
	return &Catalog{index: map[string]int{}}
}

// normSKU canonicalizes SKUs: uppercase, no spaces or dashes.
func normSKU(sku string) string {
	sku = strings.ToUpper(strings.TrimSpace(sku))
	sku = strings.ReplaceAll(sku, "-", "")
	sku = strings.ReplaceAll(sku, " ", "")
	return sku
}

// Add registers an item. In-stock items are indexed directly. Backordered
// items are also indexed so orders can still reference them, but their stock
// is clamped to zero until restock.
func (c *Catalog) Add(it Item) {
	key := normSKU(it.SKU)
	if it.Backordered {
		it.Stock = 0
		c.items = append(c.items, it)
		c.index[key] = len(c.items)
	} else {
		c.index[key] = len(c.items)
		c.items = append(c.items, it)
	}
}

// Lookup returns the position of a SKU in the catalog, or -1.
func (c *Catalog) Lookup(sku string) int {
	pos, ok := c.index[normSKU(sku)]
	if !ok {
		return -1
	}
	return pos
}

// At returns the item at a position.
func (c *Catalog) At(pos int) Item { return c.items[pos] }

// Len returns the number of items.
func (c *Catalog) Len() int { return len(c.items) }
