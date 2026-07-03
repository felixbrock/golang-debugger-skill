package billing

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// RateClient talks to the currency-rates service (see the service docs:
// GET /rate?from=X&to=Y returns the conversion rate as JSON).
type RateClient struct {
	Base string // e.g. "http://127.0.0.1:7301"
}

type rateResponse struct {
	Rate float64 `json:"rate"`
}

// Convert converts amount from one currency to another using the live rate.
func (c *RateClient) Convert(amount float64, from, to string) (float64, error) {
	url := fmt.Sprintf("%s/rate?from=%s&to=%s", c.Base, from, to)
	resp, err := http.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("rates service: %s", resp.Status)
	}
	var r rateResponse
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return 0, err
	}
	return amount * r.Rate, nil
}
