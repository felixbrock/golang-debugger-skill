// Package billing computes invoice totals across currencies, using the
// company's shared currency-rates service for conversion.
package billing

import "math"

// Line is one invoice line item.
type Line struct {
	Description string
	Amount      float64
	Currency    string
}

// InvoiceTotal converts every line to the target currency and sums them,
// rounded to cents.
func InvoiceTotal(c *RateClient, lines []Line, target string) (float64, error) {
	var total float64
	for _, ln := range lines {
		converted, err := c.Convert(ln.Amount, ln.Currency, target)
		if err != nil {
			return 0, err
		}
		total += converted
	}
	return math.Round(total*100) / 100, nil
}
